package blocklist_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/blocklist"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestExist(t *testing.T) {
	addr1 := swarm.NewAddress([]byte{0, 1, 2, 3})
	addr2 := swarm.NewAddress([]byte{4, 5, 6, 7})

	bl := blocklist.NewBlocklist(mock.NewStateStore())

	exists, err := bl.Exists(addr1)
	if err != nil {
		t.Fatal(err)
	}

	if exists {
		t.Fatal("got exists, expected not exists")
	}

	// add forever
	if err := bl.Add(addr1, 0); err != nil {
		t.Fatal(err)
	}

	// add for 50 miliseconds
	if err := bl.Add(addr2, time.Millisecond*50); err != nil {
		t.Fatal(err)
	}

	blocklist.SetTimeNow(func() time.Time { return time.Now().Add(100 * time.Millisecond) })

	exists, err = bl.Exists(addr1)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("got not exists, expected exists")
	}

	exists, err = bl.Exists(addr2)
	if err != nil {
		t.Fatal(err)
	}

	if exists {
		t.Fatal("got  exists, expected not exists")
	}

}
