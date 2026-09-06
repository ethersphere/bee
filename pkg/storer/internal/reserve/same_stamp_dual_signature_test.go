// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	batchstoremock "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	kademlia "github.com/ethersphere/bee/v2/pkg/topology/mock"
)

// TestSameStampDualSignatureLastWriteWins shows that one postage-owner key
// can produce two different protocol-valid signatures over the same stamp
// fields, and that two neighborhood nodes which receive the two resulting
// SOC versions then keep whichever payload arrived last.
func TestSameStampDualSignatureLastWriteWins(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	batchOwnerKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := crypto.NewEthereumAddress(batchOwnerKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}

	socOwnerKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	socSigner := crypto.NewDefaultSigner(socOwnerKey)

	socA := mustSOC(t, socSigner, []byte("soc-payload-A"))
	socB := mustSOC(t, socSigner, []byte("soc-payload-B"))
	if !socA.Address().Equal(socB.Address()) {
		t.Fatal("SOC chunks must share an address (same owner and id)")
	}
	if bytes.Equal(socA.Data(), socB.Data()) {
		t.Fatal("SOC chunks must wrap different payloads")
	}

	batch := postagetesting.MustNewBatch(postagetesting.WithOwner(owner))
	index := stampIndexForAddress(socA.Address(), batch.BucketDepth, 0)
	timestamp := make([]byte, 8)
	binary.BigEndian.PutUint64(timestamp, 1_000)

	digest, err := postage.ToSignDigest(socA.Address().Bytes(), batch.ID, index, timestamp)
	if err != nil {
		t.Fatal(err)
	}

	// Same private key, same digest, two random ECDSA nonces → two signatures.
	sig1 := signPostageDigestRandomK(t, batchOwnerKey, digest)
	sig2 := signPostageDigestRandomK(t, batchOwnerKey, digest)
	for bytes.Equal(sig1, sig2) {
		sig2 = signPostageDigestRandomK(t, batchOwnerKey, digest)
	}

	stamp1 := postage.NewStamp(batch.ID, index, timestamp, sig1)
	stamp2 := postage.NewStamp(batch.ID, index, timestamp, sig2)
	if !bytes.Equal(stamp1.BatchID(), stamp2.BatchID()) ||
		!bytes.Equal(stamp1.Index(), stamp2.Index()) ||
		!bytes.Equal(stamp1.Timestamp(), stamp2.Timestamp()) {
		t.Fatal("stamps must share batch, index and timestamp")
	}
	if bytes.Equal(stamp1.Sig(), stamp2.Sig()) {
		t.Fatal("signatures must differ")
	}

	hash1, err := stamp1.Hash()
	if err != nil {
		t.Fatal(err)
	}
	hash2, err := stamp2.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(hash1, hash2) {
		t.Fatal("different signatures must produce different stamp hashes")
	}

	t.Logf("same stamp fields: batch=%s index=%s timestamp=%d",
		hex.EncodeToString(batch.ID[:8]), hex.EncodeToString(index), binary.BigEndian.Uint64(timestamp))
	t.Logf("signature 1: %s", hex.EncodeToString(sig1))
	t.Logf("signature 2: %s", hex.EncodeToString(sig2))
	t.Logf("stamp hash 1: %s", hex.EncodeToString(hash1))
	t.Logf("stamp hash 2: %s", hex.EncodeToString(hash2))

	// Protocol validator used by pullsync / pushsync.
	validate := postage.ValidStamp(batchstoremock.New(batchstoremock.WithBatch(batch)))
	chunkA, err := validate(socA.WithStamp(stamp1))
	if err != nil {
		t.Fatalf("protocol rejected first signature: %v", err)
	}
	chunkB, err := validate(socB.WithStamp(stamp2))
	if err != nil {
		t.Fatalf("protocol rejected second signature: %v", err)
	}

	reserveA, storeA := newTestReservePair(t)
	reserveB, storeB := newTestReservePair(t)

	if err := reserveA.Put(ctx, chunkA); err != nil {
		t.Fatalf("reserve A put soc A: %v", err)
	}
	if err := reserveB.Put(ctx, chunkB); err != nil {
		t.Fatalf("reserve B put soc B: %v", err)
	}
	if got := chunkStoreData(t, storeA, socA.Address()); !bytes.Equal(got, socA.Data()) {
		t.Fatal("reserve A should hold soc A after the first put")
	}
	if got := chunkStoreData(t, storeB, socB.Address()); !bytes.Equal(got, socB.Data()) {
		t.Fatal("reserve B should hold soc B after the first put")
	}

	// Cross-overwrite: each node receives the other neighborhood's version.
	if err := reserveA.Put(ctx, chunkB); err != nil {
		t.Fatalf("reserve A put soc B: %v", err)
	}
	if err := reserveB.Put(ctx, chunkA); err != nil {
		t.Fatalf("reserve B put soc A: %v", err)
	}

	gotA := chunkStoreData(t, storeA, socA.Address())
	gotB := chunkStoreData(t, storeB, socB.Address())
	if !bytes.Equal(gotA, socB.Data()) {
		t.Fatal("reserve A kept the first payload; last arriving version should win")
	}
	if !bytes.Equal(gotB, socA.Data()) {
		t.Fatal("reserve B kept the first payload; last arriving version should win")
	}
	if bytes.Equal(gotA, gotB) {
		t.Fatal("neighborhoods converged; they must disagree after opposite last writes")
	}

	t.Logf("reserve A final payload matches soc B (%d bytes)", len(gotA))
	t.Logf("reserve B final payload matches soc A (%d bytes)", len(gotB))
}

func newTestReservePair(t *testing.T) (*reserve.Reserve, transaction.Storage) {
	t.Helper()
	ts := internal.NewInmemStorage()
	r, err := reserve.New(swarm.RandAddress(t), ts, 0, kademlia.NewTopologyDriver(), log.Noop)
	if err != nil {
		t.Fatal(err)
	}
	return r, ts
}

func mustSOC(t *testing.T, signer crypto.Signer, payload []byte) swarm.Chunk {
	t.Helper()
	inner, err := cac.New(payload)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := soc.New(make([]byte, swarm.HashSize), inner).Sign(signer)
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

func stampIndexForAddress(addr swarm.Address, bucketDepth uint8, collision uint32) []byte {
	bucket := binary.BigEndian.Uint32(addr.Bytes()[:4]) >> (32 - bucketDepth)
	index := make([]byte, postage.IndexSize)
	binary.BigEndian.PutUint32(index, bucket)
	binary.BigEndian.PutUint32(index[4:], collision)
	return index
}

// signPostageDigestRandomK signs ToSignDigest output the way Recover expects
// (EIP-191 prefix + keccak), but with a fresh ECDSA nonce each call.
func signPostageDigestRandomK(t *testing.T, key *ecdsa.PrivateKey, digest []byte) []byte {
	t.Helper()

	prefixed := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(digest), digest)
	hash, err := crypto.LegacyKeccak256([]byte(prefixed))
	if err != nil {
		t.Fatal(err)
	}

	//nolint:staticcheck // random nonce; Bee's SignCompact is RFC6979-deterministic
	r, s, err := ecdsa.Sign(rand.Reader, key, hash)
	if err != nil {
		t.Fatal(err)
	}

	compact := make([]byte, 65)
	r.FillBytes(compact[:32])
	s.FillBytes(compact[32:64])

	owner, err := crypto.NewEthereumAddress(key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []byte{27, 28, 29, 30} {
		compact[64] = v
		pub, err := crypto.Recover(compact, digest)
		if err != nil {
			continue
		}
		got, err := crypto.NewEthereumAddress(*pub)
		if err != nil {
			continue
		}
		if bytes.Equal(got, owner) {
			out := make([]byte, 65)
			copy(out, compact)
			return out
		}
	}
	t.Fatal("random-k signature did not recover the batch owner")
	return nil
}

func chunkStoreData(t *testing.T, store transaction.Storage, addr swarm.Address) []byte {
	t.Helper()
	ch, err := store.ChunkStore().Get(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	return ch.Data()
}
