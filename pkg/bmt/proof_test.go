package bmt_test

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"testing"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestProof(t *testing.T) {

	// BMT segment inclusion proofs
	// Usage
	buf := make([]byte, 4096)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		t.Fatal(err)
	}

	pool := bmt.NewPool(bmt.NewConf(swarm.NewHasher, 128, 128))
	hh := pool.Get()
	defer pool.Put(hh)
	hh.SetHeaderInt64(4096)

	_, err = hh.Write(buf)
	if err != nil {
		t.Fatal(err)
	}

	rh, err := hh.Hash(nil)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 128; i++ {
		t.Run(fmt.Sprintf("segmentIndex %d", i), func(t *testing.T) {
			proof := bmt.Prover{hh}.Proof(i)

			h := pool.Get()
			defer pool.Put(h)

			root, err := bmt.Prover{h}.Verify(i, proof)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(rh, root) {
				t.Fatalf("incorrect hash. wanted %x, got %x.", rh, root)
			}
		})
	}
}
