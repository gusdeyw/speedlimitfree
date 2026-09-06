package traffic

import (
	"encoding/binary"
	"testing"
)

func TestPacketDirectionAndFragments(t *testing.T) {
	b := make([]byte, 40)
	b[0] = 0x45
	b[9] = 6
	copy(b[12:16], []byte{192, 168, 1, 2})
	copy(b[16:20], []byte{1, 1, 1, 1})
	binary.BigEndian.PutUint16(b[20:22], 4000)
	binary.BigEndian.PutUint16(b[22:24], 443)
	k, ok := Parse(b, true)
	if !ok || k.Local.String() != "192.168.1.2:4000" || k.Remote.String() != "1.1.1.1:443" {
		t.Fatalf("bad tuple: %+v", k)
	}
	in, _ := Parse(b, false)
	if in.Local != k.Remote {
		t.Fatal("inbound tuple not reversed")
	}
	b[6] = 0x20
	if _, ok = Parse(b, true); ok {
		t.Fatal("fragment attributed")
	}
	for n := 0; n < 24; n++ {
		if _, ok = Parse(b[:n], true); ok {
			t.Fatalf("truncated packet length %d accepted", n)
		}
	}
}
func TestIPv6Extensions(t *testing.T) {
	b := make([]byte, 56)
	b[0] = 0x60
	b[6] = 0
	b[23] = 1
	b[39] = 2
	b[40] = 17
	b[41] = 0
	binary.BigEndian.PutUint16(b[48:50], 5353)
	binary.BigEndian.PutUint16(b[50:52], 443)
	k, ok := Parse(b, true)
	if !ok || k.Protocol != 17 || k.Local.Port() != 5353 {
		t.Fatal("extension chain was not parsed")
	}
	b[6] = 44
	if _, ok = Parse(b, true); ok {
		t.Fatal("IPv6 fragment accepted")
	}
}
func TestWinDivertAddressByteOrder(t *testing.T) {
	b := []byte{1, 1, 1, 1, 255, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if ip := flowAddress(b); ip.String() != "1.1.1.1" {
		t.Fatalf("wrong host-order IPv4 mapping: %s", ip)
	}
}
