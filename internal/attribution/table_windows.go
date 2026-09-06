package attribution

import (
	"encoding/binary"
	"fmt"
	"golang.org/x/sys/windows"
	"net/netip"
	"speedlimitfree/internal/contracts"
	"speedlimitfree/internal/processes"
	"speedlimitfree/internal/traffic"
	"sync"
	"unsafe"
)

type owner struct {
	Process   contracts.Process
	Endpoint  uint64
	Ambiguous bool
}
type Table struct {
	mu            sync.RWMutex
	events, seeds map[traffic.Key]owner
	live          map[uint32]contracts.Process
}

func New() *Table {
	return &Table{events: map[traffic.Key]owner{}, seeds: map[traffic.Key]owner{}, live: map[uint32]contracts.Process{}}
}
func (t *Table) Event(e traffic.FlowEvent) {
	if e.Deleted {
		t.mu.Lock()
		if old, ok := t.events[e.Key]; ok && old.Endpoint == e.Endpoint {
			delete(t.events, e.Key)
		}
		t.mu.Unlock()
		return
	}
	p, err := processes.Get(e.PID)
	if err != nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.events) >= 100000 {
		return
	}
	old, ok := t.events[e.Key]
	ambiguous := ok && old.Process.PID != p.PID
	t.events[e.Key] = owner{Process: p, Endpoint: e.Endpoint, Ambiguous: ambiguous}
	t.live[p.PID] = p
}
func (t *Table) Refresh(ps []contracts.Process) error {
	live := map[uint32]contracts.Process{}
	for _, p := range ps {
		if p.Started != "" {
			live[p.PID] = p
		}
	}
	seeds := map[traffic.Key]owner{}
	var firstErr error
	for _, tcp := range []bool{true, false} {
		for _, ipv6 := range []bool{false, true} {
			if err := readTable(tcp, ipv6, func(k traffic.Key, pid uint32) {
				p, ok := live[pid]
				if !ok {
					return
				}
				old, exists := seeds[k]
				seeds[k] = owner{Process: p, Ambiguous: exists && (old.Ambiguous || old.Process.PID != pid)}
			}); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	t.mu.Lock()
	t.live = live
	t.seeds = seeds
	for k, o := range t.events {
		p, ok := live[o.Process.PID]
		if !ok || p.Started != o.Process.Started {
			delete(t.events, k)
		}
	}
	t.mu.Unlock()
	return firstErr
}
func (t *Table) Lookup(k traffic.Key) (contracts.Process, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if o, ok := t.events[k]; ok {
		return o.Process, !o.Ambiguous
	}
	if o, ok := t.seeds[k]; ok {
		return o.Process, !o.Ambiguous
	}
	if k.Protocol == 17 {
		k.Remote = netip.AddrPort{}
		if o, ok := t.seeds[k]; ok {
			return o.Process, !o.Ambiguous
		}
		ip := netip.IPv4Unspecified()
		if k.Local.Addr().Is6() {
			ip = netip.IPv6Unspecified()
		}
		k.Local = netip.AddrPortFrom(ip, k.Local.Port())
		if o, ok := t.seeds[k]; ok {
			return o.Process, !o.Ambiguous
		}
	}
	return contracts.Process{}, false
}

var iphelper = windows.NewLazySystemDLL("iphlpapi.dll")

func readTable(tcp, ipv6 bool, add func(traffic.Key, uint32)) error {
	name := "GetExtendedUdpTable"
	class := uintptr(1)
	if tcp {
		name = "GetExtendedTcpTable"
		class = 5
	}
	af := uintptr(2)
	if ipv6 {
		af = 23
	}
	proc := iphelper.NewProc(name)
	var size uint32
	code, _, _ := proc.Call(0, uintptr(unsafe.Pointer(&size)), 0, af, class, 0)
	if code != 0 && code != 122 {
		return fmt.Errorf("%s: %d", name, code)
	}
	for attempt := 0; attempt < 3; attempt++ {
		if size < 4 || size > 8<<20 {
			return fmt.Errorf("invalid connection table size %d", size)
		}
		buf := make([]byte, size)
		code, _, _ = proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, af, class, 0)
		if code == 122 {
			continue
		}
		if code != 0 {
			return fmt.Errorf("%s: %d", name, code)
		}
		stride := 12
		if tcp {
			stride = 24
		}
		if ipv6 {
			stride = 28
			if tcp {
				stride = 56
			}
		}
		count := int(binary.LittleEndian.Uint32(buf[:4]))
		if count > (len(buf)-4)/stride {
			return fmt.Errorf("invalid connection table row count")
		}
		for i := 0; i < count; i++ {
			b := buf[4+i*stride : 4+(i+1)*stride]
			k := traffic.Key{Protocol: 17}
			var pid uint32
			if tcp {
				k.Protocol = 6
			}
			if ipv6 {
				// Link-local scoped addresses are deliberately left unknown.
				if binary.LittleEndian.Uint32(b[16:20]) != 0 {
					continue
				}
				k.Local = netip.AddrPortFrom(netip.AddrFrom16([16]byte(b[:16])).Unmap(), binary.BigEndian.Uint16(b[20:22]))
				pid = binary.LittleEndian.Uint32(b[24:28])
				if tcp {
					if binary.LittleEndian.Uint32(b[40:44]) != 0 {
						continue
					}
					k.Remote = netip.AddrPortFrom(netip.AddrFrom16([16]byte(b[24:40])).Unmap(), binary.BigEndian.Uint16(b[44:46]))
					pid = binary.LittleEndian.Uint32(b[52:56])
				}
			} else {
				offset := 0
				if tcp {
					offset = 4
				}
				k.Local = netip.AddrPortFrom(netip.AddrFrom4([4]byte(b[offset:offset+4])), binary.BigEndian.Uint16(b[offset+4:offset+6]))
				pid = binary.LittleEndian.Uint32(b[8:12])
				if tcp {
					k.Remote = netip.AddrPortFrom(netip.AddrFrom4([4]byte(b[12:16])), binary.BigEndian.Uint16(b[16:18]))
					pid = binary.LittleEndian.Uint32(b[20:24])
				}
			}
			add(k, pid)
		}
		return nil
	}
	return fmt.Errorf("connection table changed repeatedly")
}
