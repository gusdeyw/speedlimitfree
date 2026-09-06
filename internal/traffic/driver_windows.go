package traffic

import (
	"encoding/binary"
	"fmt"
	"golang.org/x/sys/windows"
	"net/netip"
	"path/filepath"
	"sync"
	"unsafe"
)

type FlowEvent struct {
	Key      Key
	PID      uint32
	Endpoint uint64
	Deleted  bool
}
type Driver struct {
	dll                         *windows.DLL
	recv, send, close, shutdown *windows.Proc
	network, flow, socket       uintptr
	once                        sync.Once
	pool                        sync.Pool
}

func Open(directory string) (*Driver, error) {
	path, err := filepath.Abs(filepath.Join(directory, "WinDivert.dll"))
	if err != nil {
		return nil, err
	}
	dll, err := windows.LoadDLL(path)
	if err != nil {
		return nil, fmt.Errorf("load WinDivert: %w", err)
	}
	d := &Driver{dll: dll}
	d.pool.New = func() any { return make([]byte, 65575) }
	open, err := dll.FindProc("WinDivertOpen")
	if err != nil {
		dll.Release()
		return nil, err
	}
	for name, dst := range map[string]**windows.Proc{"WinDivertRecv": &d.recv, "WinDivertSend": &d.send, "WinDivertClose": &d.close, "WinDivertShutdown": &d.shutdown} {
		*dst, err = dll.FindProc(name)
		if err != nil {
			dll.Release()
			return nil, err
		}
	}
	create := func(filter string, layer, flags uintptr) (uintptr, error) {
		f, _ := windows.BytePtrFromString(filter)
		h, _, e := open.Call(uintptr(unsafe.Pointer(f)), layer, 0, flags)
		if h == ^uintptr(0) {
			return 0, e
		}
		return h, nil
	}
	// Observe connection establishment before diverting any network packets.
	d.flow, err = create("true", 2, 1|4)
	if err != nil {
		d.Close()
		return nil, fmt.Errorf("open flow observer (administrator required): %w", err)
	}
	d.socket, err = create("event == CONNECT or event == ACCEPT", 3, 1|4)
	if err != nil {
		d.Close()
		return nil, fmt.Errorf("open socket observer: %w", err)
	}
	d.network, err = create("!loopback and !impostor and (tcp or udp)", 0, 0)
	if err != nil {
		d.Close()
		return nil, fmt.Errorf("open network capture: %w", err)
	}
	return d, nil
}

func (d *Driver) Recv() (*Packet, error) {
	b := d.pool.Get().([]byte)
	p := &Packet{Data: b}
	var n uint32
	ok, _, err := d.recv.Call(d.network, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&p.Address[0])))
	if ok == 0 {
		d.pool.Put(b)
		return nil, err
	}
	p.Data = b[:n]
	p.Outbound = p.Address[10]&2 != 0
	p.Key, p.Known = Parse(p.Data, p.Outbound)
	return p, nil
}
func (d *Driver) Send(p *Packet) error {
	var n uint32
	if len(p.Data) == 0 {
		return nil
	}
	ok, _, err := d.send.Call(d.network, uintptr(unsafe.Pointer(&p.Data[0])), uintptr(len(p.Data)), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&p.Address[0])))
	if ok == 0 {
		return err
	}
	if int(n) != len(p.Data) {
		return fmt.Errorf("short packet injection: %d/%d", n, len(p.Data))
	}
	return nil
}
func (d *Driver) Release(p *Packet) {
	if cap(p.Data) >= 65575 {
		d.pool.Put(p.Data[:65575])
	}
	p.Data = nil
}

func (d *Driver) Observe(socket bool, notify func(FlowEvent)) error {
	h := d.flow
	if socket {
		h = d.socket
	}
	for {
		var a [80]byte
		ok, _, err := d.recv.Call(h, 0, 0, 0, uintptr(unsafe.Pointer(&a[0])))
		if ok == 0 {
			return err
		}
		f := a[16:]
		event := FlowEvent{PID: binary.LittleEndian.Uint32(f[16:20]), Endpoint: binary.LittleEndian.Uint64(f[:8]), Deleted: a[9] == 2}
		event.Key = Key{Local: netip.AddrPortFrom(flowAddress(f[20:36]), binary.LittleEndian.Uint16(f[52:54])), Remote: netip.AddrPortFrom(flowAddress(f[36:52]), binary.LittleEndian.Uint16(f[54:56])), Protocol: f[56]}
		notify(event)
	}
}
func flowAddress(b []byte) netip.Addr {
	var ip [16]byte
	for i := 0; i < 16; i++ {
		ip[i] = b[15-i]
	}
	return netip.AddrFrom16(ip).Unmap()
}
func (d *Driver) StopReceive() {
	for _, h := range []uintptr{d.network, d.flow, d.socket} {
		if h != 0 {
			d.shutdown.Call(h, 1)
		}
	}
}
func (d *Driver) Close() {
	d.once.Do(func() {
		if d.shutdown != nil {
			d.StopReceive()
		}
		if d.close != nil {
			for _, h := range []uintptr{d.network, d.flow, d.socket} {
				if h != 0 {
					d.close.Call(h)
				}
			}
		}
	})
}
