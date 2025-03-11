package kademlia_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	cryptomock "github.com/ethersphere/bee/v2/pkg/crypto/mock"
	"github.com/ethersphere/bee/v2/pkg/topology/kademlia"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
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

	var kads []*kademlia.Kad

	for range 100 {
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

	for i, k1 := range kads {
		for j, k2 := range kads {
			if i == j {
				continue
			}
			addOne(t, defaultSigner(), k1, k1.AdressBook(), k2.Address())
		}
		fmt.Println(i)
	}

	for _, k := range kads {
		p, _ := json.MarshalIndent(k.Snapshot(), "", "\t")
		fmt.Println(string(p))
	}
}
