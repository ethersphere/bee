package kademlia

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/swarm"
)

func TestWaitList(t *testing.T) {
	var (
		wl   = new(waitList)
		addr = swarm.MustParseHexAddress("123456")
		now  = time.Now()
	)

	wl.setPeerExpire(addr, now.Add(1*time.Minute))
	if have, want := wl.isPeerBeforeExpire(addr), true; have != want {
		t.Fatalf("isPeerBeforeExpire(%q): have %t; want %t", addr, have, want)
	}

	wl.setPeerInfo(addr, now.Add(-1*time.Minute), 2)
	if have, want := wl.isPeerBeforeExpire(addr), false; have != want {
		t.Fatalf("isPeerBeforeExpire(%q): have %t; want %t", addr, have, want)
	}
	if have, want := wl.peerFailedConnectionAttempts(addr), 2; have != want {
		t.Fatalf("peerFailedConnectionAttempts(%q): have %d; want %d", addr, have, want)
	}

	wl.setPeerExpire(addr, now.Add(1*time.Minute))
	if have, want := wl.isPeerBeforeExpire(addr), true; have != want {
		t.Fatalf("isPeerBeforeExpire(%q): have %t; want %t", addr, have, want)
	}

	wl.removePeer(addr)
	if have, want := wl.isPeerBeforeExpire(addr), false; have != want {
		t.Fatalf("isPeerBeforeExpire(%q): have %t; want %t", addr, have, want)
	}
	if have, want := wl.peerFailedConnectionAttempts(addr), 0; have != want {
		t.Fatalf("peerFailedConnectionAttempts(%q): have %d; want %d", addr, have, want)
	}
}
