package cli

import (
	"fmt"
	"net"
	"testing"
)

func TestListenLoopbackUsesPreferredPort(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()

	ln, err := listenLoopback(port)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if got := ln.Addr().(*net.TCPAddr).Port; got != port {
		t.Fatalf("got port %d, want preferred %d", got, port)
	}
}

func TestListenLoopbackSkipsOccupiedPorts(t *testing.T) {
	start, occupied := occupyLoopbackRange(t, 2)
	defer closeListeners(occupied)

	ln, err := listenLoopback(start)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := ln.Addr().(*net.TCPAddr).Port
	if got != start+2 {
		t.Fatalf("got port %d, want next free %d", got, start+2)
	}
}

func TestListenLoopbackSkipsWildcardOccupiedPort(t *testing.T) {
	start, occupied := occupyAddressRange(t, "0.0.0.0", 1)
	defer closeListeners(occupied)

	ln, err := listenLoopback(start)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := ln.Addr().(*net.TCPAddr).Port
	if got == start {
		t.Fatalf("bound 127.0.0.1:%d while 0.0.0.0:%d is already listening", got, start)
	}
	if got != start+1 {
		t.Fatalf("got port %d, want next free %d", got, start+1)
	}
}

func TestListenLoopbackPortZero(t *testing.T) {
	ln, err := listenLoopback(0)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if ln.Addr().(*net.TCPAddr).Port == 0 {
		t.Fatal("port 0 did not receive a kernel-assigned port")
	}
}

func occupyLoopbackRange(t *testing.T, n int) (int, []net.Listener) {
	t.Helper()
	return occupyAddressRange(t, "127.0.0.1", n)
}

func occupyAddressRange(t *testing.T, host string, n int) (int, []net.Listener) {
	t.Helper()
	for start := 20000; start < 45000; start++ {
		var lns []net.Listener
		ok := true
		for i := 0; i < n; i++ {
			ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, start+i))
			if err != nil {
				closeListeners(lns)
				ok = false
				break
			}
			lns = append(lns, ln)
		}
		if !ok {
			continue
		}
		probe, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", start+n))
		if err != nil {
			closeListeners(lns)
			continue
		}
		probe.Close()
		return start, lns
	}
	t.Fatalf("no free consecutive ports for %s", host)
	return 0, nil
}

func closeListeners(lns []net.Listener) {
	for _, ln := range lns {
		ln.Close()
	}
}
