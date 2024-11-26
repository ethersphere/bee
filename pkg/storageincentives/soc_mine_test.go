// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives_test

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"math/big"
	"os"
	"sync"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bmt"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

// TestSocMine dumps a sample made out SOCs to upload for storage incestives

// dump chunks

// go test -v ./pkg/storageincentives/ -run TestSocMine -count 1 > socs.txt

// to generate uploads using the input
// cat socs.txt | tail 19 | head 16 | perl -pne 's/([a-f0-9]+)\t([a-f0-9]+)\t([a-f0-9]+)\t([a-f0-9]+)/echo -n $4 | xxd -r -p | curl -X POST \"http:\/\/localhost:1633\/soc\/$1\/$2?sig=$3\" -H \"accept: application\/json, text\/plain, \/\" -H \"content-type: application\/octet-stream\" -H \"swarm-postage-batch-id: 14b26beca257e763609143c6b04c2c487f01a051798c535c2f542ce75a97c05f\" --data-binary \@-/'
func TestSocMine(t *testing.T) {
	t.Parallel()
	// the anchor used in neighbourhood selection and reserve salt for sampling
	prefix, err := hex.DecodeString("3617319a054d772f909f7c479a2cebe5066e836a939412e32403c99029b92eff")
	if err != nil {
		t.Fatal(err)
	}
	// the transformed address hasher factory function
	prefixhasher := func() hash.Hash { return swarm.NewPrefixHasher(prefix) }
	trHasher := func() hash.Hash { return bmt.NewHasher(prefixhasher) }
	// the bignum cast of the maximum sample value (upper bound on transformed addresses as a 256-bit article)
	// this constant is for a minimum reserve size of 2 million chunks with sample size of 16
	// = 1.284401 * 10^71 = 1284401 + 66 0-s
	mstring := "1284401"
	for i := 0; i < 66; i++ {
		mstring = mstring + "0"
	}
	n, ok := new(big.Int).SetString(mstring, 10)
	if !ok {
		t.Fatalf("SetString: error setting to '%s'", mstring)
	}
	// the filter function on the SOC address
	// meant to make sure we pass check for proof of retrievability for
	// a node of overlay 0x65xxx with a reserve depth of 1, i.e.,
	// SOC address must start with zero bit
	filterSOCAddr := func(a swarm.Address) bool {
		return a.Bytes()[0]&0x80 != 0x00
	}
	// the filter function on the transformed address using the density estimation constant
	filterTrAddr := func(a swarm.Address) (bool, error) {
		m := new(big.Int).SetBytes(a.Bytes())
		return m.Cmp(n) < 0, nil
	}
	// setup the signer with a private key from a fixture
	data, err := hex.DecodeString("634fb5a872396d9693e5c9f9d7233cfa93f395c093371017ff44aa9ae6564cdd")
	if err != nil {
		t.Fatal(err)
	}
	privKey, err := crypto.DecodeSecp256k1PrivateKey(data)
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)

	sampleSize := 16
	// for sanity check: given a filterSOCAddr requiring a 0 leading bit (chance of 1/2)
	// we expect an overall rough 4 million chunks to be mined to create this sample
	// for 8 workers that is half a million round on average per worker
	err = makeChunks(t, signer, sampleSize, filterSOCAddr, filterTrAddr, trHasher)
	if err != nil {
		t.Fatal(err)
	}
}

func makeChunks(t *testing.T, signer crypto.Signer, sampleSize int, filterSOCAddr func(swarm.Address) bool, filterTrAddr func(swarm.Address) (bool, error), trHasher func() hash.Hash) error {
	t.Helper()

	// set owner address from signer
	ethAddress, err := signer.EthereumAddress()
	if err != nil {
		return err
	}
	ownerAddressBytes := ethAddress.Bytes()

	// use the same wrapped chunk for all mined SOCs
	payload := []byte("foo")
	ch, err := cac.New(payload)
	if err != nil {
		return err
	}

	var done bool                          // to signal sampleSize number of chunks found
	sampleC := make(chan *soc.SOC, 1)      // channel to push results on
	sample := make([]*soc.SOC, sampleSize) // to collect the sample
	ctx, cancel := context.WithCancel(context.Background())
	eg, ectx := errgroup.WithContext(ctx)
	// the main loop terminating after sampleSize SOCs have been generated
	eg.Go(func() error {
		defer cancel()
		for i := 0; i < sampleSize; i++ {
			select {
			case sample[i] = <-sampleC:
			case <-ectx.Done():
				return ectx.Err()
			}
		}
		done = true
		return nil
	})

	// loop to start mining workers
	count := 8 // number of parallel workers
	wg := sync.WaitGroup{}
	for i := 0; i < count; i++ {
		wg.Add(1)
		eg.Go(func() (err error) {
			offset := i * 4
			found := 0
			for seed := uint32(1); ; seed++ {
				select {
				case <-ectx.Done():
					defer wg.Done()
					t.Logf("LOG quit worker: %d, rounds: %d, found: %d\n", i, seed, found)
					return ectx.Err()
				default:
				}
				id := make([]byte, 32)
				binary.BigEndian.PutUint32(id[offset:], seed)
				s := soc.New(id, ch)
				addr, err := soc.CreateAddress(id, ownerAddressBytes)
				if err != nil {
					return err
				}
				// continue if mined SOC addr is not good
				if !filterSOCAddr(addr) {
					continue
				}
				hasher := trHasher()
				data := s.WrappedChunk().Data()
				hasher.(*bmt.Hasher).SetHeader(data[:8])
				_, err = hasher.Write(data[8:])
				if err != nil {
					return err
				}
				trAddr := hasher.Sum(nil)
				// hashing the transformed wrapped chunk address with the SOC address
				// to arrive at a unique transformed SOC address despite identical payloads
				trSocAddr, err := soc.CreateAddress(addr.Bytes(), trAddr)
				if err != nil {
					return err
				}
				ok, err := filterTrAddr(trSocAddr)
				if err != nil {
					return err
				}
				if ok {
					select {
					case sampleC <- s:
						found++
						t.Logf("LOG worker: %d, rounds: %d, found: %d, id:%x\n", i, seed, found, id)
					case <-ectx.Done():
						defer wg.Done()
						t.Logf("LOG quit worker: %d, rounds: %d, found: %d\n", i, seed, found)
						return ectx.Err()
					}
				}
			}
		})
	}
	if err := eg.Wait(); !done && err != nil {
		return err
	}
	wg.Wait()
	for _, s := range sample {

		// signs the chunk
		sch, err := s.Sign(signer)
		if err != nil {
			return err
		}
		data := sch.Data()
		id, sig, payload := data[:32], data[32:97], data[97:]
		fmt.Fprintf(os.Stdout, "%x\t%x\t%x\t%x\n", ownerAddressBytes, id, sig, payload)

	}
	return nil
}
