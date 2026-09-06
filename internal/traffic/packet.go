package traffic

import (
	"encoding/binary"
	"net/netip"
)

type Key struct {
	Local    netip.AddrPort
	Remote   netip.AddrPort
	Protocol uint8
}
type Packet struct {
	Data     []byte
	Address  [80]byte
	Key      Key
	Outbound bool
	Known    bool
}

// Parse leaves fragments and unsupported extension chains unattributed.
func Parse(data []byte, outbound bool) (Key, bool) {
	var src, dst netip.Addr
	var proto uint8
	offset := 0
	if len(data) < 20 {
		return Key{}, false
	}
	switch data[0] >> 4 {
	case 4:
		offset = int(data[0]&15) * 4
		if offset < 20 || len(data) < offset+4 || binary.BigEndian.Uint16(data[6:8])&0x3fff != 0 {
			return Key{}, false
		}
		proto = data[9]
		src = netip.AddrFrom4([4]byte(data[12:16]))
		dst = netip.AddrFrom4([4]byte(data[16:20]))
	case 6:
		if len(data) < 40 {
			return Key{}, false
		}
		src = netip.AddrFrom16([16]byte(data[8:24])).Unmap()
		dst = netip.AddrFrom16([16]byte(data[24:40])).Unmap()
		proto = data[6]
		offset = 40
		for count := 0; proto != 6 && proto != 17; count++ {
			if count >= 8 || len(data) < offset+2 {
				return Key{}, false
			}
			next := data[offset]
			n := 0
			switch proto {
			case 0, 43, 60:
				n = (int(data[offset+1]) + 1) * 8
			case 51:
				n = (int(data[offset+1]) + 2) * 4
			default:
				return Key{}, false
			}
			if n < 8 || offset+n > len(data) {
				return Key{}, false
			}
			offset += n
			proto = next
		}
	default:
		return Key{}, false
	}
	if (proto != 6 && proto != 17) || len(data) < offset+4 {
		return Key{}, false
	}
	if src.IsLinkLocalUnicast() || dst.IsLinkLocalUnicast() {
		return Key{}, false
	}
	a := netip.AddrPortFrom(src, binary.BigEndian.Uint16(data[offset:offset+2]))
	b := netip.AddrPortFrom(dst, binary.BigEndian.Uint16(data[offset+2:offset+4]))
	if !outbound {
		a, b = b, a
	}
	return Key{Local: a, Remote: b, Protocol: proto}, true
}
