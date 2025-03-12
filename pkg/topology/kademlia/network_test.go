package kademlia_test

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	cryptomock "github.com/ethersphere/bee/v2/pkg/crypto/mock"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/topology/kademlia"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"golang.org/x/sync/errgroup"
)

var (
	defaultSigner = func() crypto.Signer {
		return cryptomock.New(cryptomock.WithSignFunc(func(a []byte) ([]byte, error) {
			key, _ := crypto.GenerateSecp256k1Key()
			signer := crypto.NewDefaultSigner(key)
			signature, _ := signer.Sign(a)
			return signature, nil
		}))
	}
)

/*
Network tests are meant to help engineers run large scale, close-to-reality experiments to simuluate network behavior under different circumstances by directly using the kademlia package.

The idea is to spin off many kademlia instances and connect them to each other, with varying
levels of things like peer latency, peer reachability, etc, and observe different network behaviors like total number of hops for a pushsync request, total request time as a sum of peer latencies, and routes taken to the closest peer.

The end goal is to help engineers understand the different behaviors, design better protocols, and fine-tune network performance altering parameters of things like salud health percentile thresholds.
*/

func TestNetwork(t *testing.T) {

	count := 1000
	kads := make([]*kademlia.Kad, 0, count)

	for range count {
		var (
			conns           int32
			_, kad, _, _, _ = newTestKademlia(t, &conns, nil, kademlia.Options{})
		)

		kads = append(kads, kad)
		if err := kad.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		testutil.CleanupCloser(t, kad)
		kad.SetStorageRadius(11)
	}

	var eg errgroup.Group
	eg.SetLimit(runtime.NumCPU())

	for i, k1 := range kads {
		eg.Go(func() error {
			for j, k2 := range kads {
				if i == j {
					continue
				}
				if j%2 == 0 {
					addOne(t, defaultSigner(), k1, k1.AdressBook(), k2.Address())
				} else {
					connectOne(t, defaultSigner(), k1, k1.AdressBook(), k2.Address(), nil)
				}
				k1.Reachable(k2.Address(), p2p.ReachabilityStatusPublic)
				k1.UpdatePeerHealth(k2.Address(), true, time.Second)
			}
			fmt.Println(i)
			return nil
		})
	}
	err := eg.Wait()
	if err != nil {
		t.Fatal(err)
	}

	p, _ := json.MarshalIndent(kads[0].Snapshot(), "", "\t")
	fmt.Println(string(p))

	a := swarm.RandAddress(t)
	closest, err := kads[0].ClosestPeer(a, false, topology.Select{})
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("closest", a, closest)
}
