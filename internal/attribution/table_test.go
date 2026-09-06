package attribution

import (
	"net"
	"net/netip"
	"os"
	"speedlimitfree/internal/processes"
	"speedlimitfree/internal/traffic"
	"testing"
)

func TestSeedExistingUDPProcessOwnership(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ps, err := processes.List()
	if err != nil {
		t.Fatal(err)
	}
	table := New()
	if err = table.Refresh(ps); err != nil {
		t.Fatal(err)
	}
	local := netip.MustParseAddrPort(conn.LocalAddr().String())
	p, ok := table.Lookup(traffic.Key{Local: local, Remote: netip.MustParseAddrPort("1.1.1.1:443"), Protocol: 17})
	if !ok || p.PID != uint32(os.Getpid()) || p.Started == "" {
		t.Fatalf("existing UDP owner not resolved: %+v", p)
	}
}
