package libp2p

import (
	"testing"

	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func Test(t *testing.T) {
	badPortObservable, err := ma.NewMultiaddr("/ip4/100.20.0.1/tcp/7070/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkS")
	if err != nil {
		t.Fatal(err)
	}

	badPorObservableAddrInfo, err := libp2ppeer.AddrInfoFromP2pAddr(badPortObservable)
	if err != nil {
		t.Fatal(err)
	}

	observable, err := ma.NewMultiaddr("/ip4/80.20.0.1/tcp/3030/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkS")
	if err != nil {
		t.Fatal(err)
	}

	node1ma, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/7070")
	if err != nil {
		t.Fatal(err)
	}

	node2ma, err := ma.NewMultiaddr("/ip4/100.20.0.1/tcp/5555")
	if err != nil {
		t.Fatal(err)
	}

	node3ma, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/7070")
	if err != nil {
		t.Fatal(err)
	}

	a, err := advertisableAddress(badPortObservable, []ma.Multiaddr{node1ma, node2ma, node3ma})
	if err != nil {
		t.Fatal(err)
	}

	expected, err := buildUnderlayAddress(node2ma, badPorObservableAddrInfo.ID)
	if err != nil {
		t.Fatal(err)
	}

	if !a.Equal(expected) {
		t.Fatalf("expected %s, got %s", expected.String(), a.String())
	}

	a, err = advertisableAddress(observable, []ma.Multiaddr{node1ma, node2ma, node3ma})
	if err != nil {
		t.Fatal(err)
	}

	if !a.Equal(observable) {
		t.Fatalf("expected %s, got %s", observable.String(), a.String())
	}
}
