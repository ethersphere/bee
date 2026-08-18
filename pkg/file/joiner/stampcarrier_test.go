// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package joiner_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/encryption"
	dec "github.com/ethersphere/bee/v2/pkg/encryption/store"
	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/file/joiner"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy/getter"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy/stampcarrier"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

// stampingStore is an in-memory chunk store whose Put stamps every chunk with
// a real signature, mimicking the api putterSessionWrapper, and records the
// original stamp bytes per chunk address for later assertions.
type stampingStore struct {
	storage.ChunkStore
	stamper postage.Stamper
	mu      sync.Mutex
	stamps  map[string][]byte // chunk address -> original marshaled stamp
}

// newStampingStore returns the store and the batch owner's ethereum address.
func newStampingStore(t *testing.T) (*stampingStore, []byte) {
	t.Helper()
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := crypto.NewEthereumAddress(privKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)
	batchID := make([]byte, 32)
	if _, err := rand.Read(batchID); err != nil {
		t.Fatal(err)
	}
	// depth 24 / bucket depth 16 gives 256 slots per collision bucket,
	// plenty for the few thousand chunks the tests upload
	issuer := postage.NewStampIssuer("label", "keyID", batchID, big.NewInt(3), 24, 16, 0, true)
	return &stampingStore{
		ChunkStore: inmemchunkstore.New(),
		stamper:    postage.NewStamper(inmemstore.New(), issuer, signer),
		stamps:     make(map[string][]byte),
	}, owner
}

func (s *stampingStore) Put(ctx context.Context, ch swarm.Chunk) error {
	idAddr, err := storage.IdentityAddress(ch)
	if err != nil {
		return err
	}
	stamp, err := s.stamper.Stamp(ch.Address(), idAddr)
	if err != nil {
		return err
	}
	ch = ch.WithStamp(stamp)
	b, err := stamp.MarshalBinary()
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.stamps[ch.Address().ByteString()] = b
	s.mu.Unlock()
	return s.ChunkStore.Put(ctx, ch)
}

func (s *stampingStore) originalStamp(addr swarm.Address) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stamps[addr.ByteString()]
}

func uploadFile(t *testing.T, st storage.Putter, dataLen int, encrypt bool) (swarm.Address, []byte) {
	t.Helper()
	data := testutil.RandBytes(t, dataLen)
	pipe := builder.NewPipelineBuilder(context.Background(), st, encrypt, redundancy.MEDIUM)
	root, err := builder.FeedPipeline(context.Background(), pipe, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return root, data
}

// parentPayload fetches (and for encrypted refs decrypts) an intermediate
// chunk and returns its payload without the span.
func parentPayload(t *testing.T, st storage.ChunkStore, ref swarm.Address) []byte {
	t.Helper()
	ch, err := st.Get(context.Background(), swarm.NewAddress(ref.Bytes()[:swarm.HashSize]))
	if err != nil {
		t.Fatal(err)
	}
	data := ch.Data()
	if len(ref.Bytes()) == encryption.ReferenceSize {
		data, err = dec.DecryptChunkData(data, ref.Bytes()[swarm.HashSize:])
		if err != nil {
			t.Fatal(err)
		}
	}
	return data[swarm.SpanSize:]
}

func readAll(t *testing.T, ctx context.Context, st storage.ChunkStore, putter storage.Putter, root swarm.Address) []byte {
	t.Helper()
	j, _, err := joiner.New(ctx, st, putter, root, redundancy.MEDIUM)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := file.JoinReadAll(ctx, j, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// checkCarriers parses the trailing carrier group of a parent payload and
// asserts each child's stamp is present, byte-identical to the original.
func checkCarriers(t *testing.T, st *stampingStore, payload []byte, refLen, m, k, c int) {
	t.Helper()
	children := m + k
	carrierRefs := c + stampcarrier.GroupParities
	wantLen := m*refLen + (k+carrierRefs)*swarm.HashSize
	if len(payload) != wantLen {
		t.Fatalf("parent payload length: got %d, want %d", len(payload), wantLen)
	}

	// collect child addresses by slot in [data ‖ parity]
	childAddrs := make([]swarm.Address, 0, children)
	for i := range m {
		childAddrs = append(childAddrs, swarm.NewAddress(payload[i*refLen:i*refLen+swarm.HashSize]))
	}
	for i := range k {
		off := m*refLen + i*swarm.HashSize
		childAddrs = append(childAddrs, swarm.NewAddress(payload[off:off+swarm.HashSize]))
	}

	// fetch carrier chunks and unpack all entries
	entries := make(map[uint16][]byte)
	for i := range c {
		off := m*refLen + (k+i)*swarm.HashSize
		addr := swarm.NewAddress(payload[off : off+swarm.HashSize])
		ch, err := st.Get(context.Background(), addr)
		if err != nil {
			t.Fatalf("carrier %d not stored: %v", i, err)
		}
		if len(ch.Data()) != swarm.ChunkWithSpanSize {
			t.Fatalf("carrier %d not padded: %d", i, len(ch.Data()))
		}
		unpacked, err := stampcarrier.Unpack(ch.Data()[swarm.SpanSize:])
		if err != nil {
			t.Fatal(err)
		}
		for idx, s := range unpacked {
			entries[idx] = s
		}
	}
	// carrier parities must be stored too
	for i := range stampcarrier.GroupParities {
		off := m*refLen + (k+c+i)*swarm.HashSize
		addr := swarm.NewAddress(payload[off : off+swarm.HashSize])
		if _, err := st.Get(context.Background(), addr); err != nil {
			t.Fatalf("carrier parity %d not stored: %v", i, err)
		}
	}

	if len(entries) != children {
		t.Fatalf("carrier entries: got %d, want %d", len(entries), children)
	}
	for i, addr := range childAddrs {
		want := st.originalStamp(addr)
		if want == nil {
			t.Fatalf("no original stamp recorded for child %d", i)
		}
		if !bytes.Equal(entries[uint16(i)], want) {
			t.Fatalf("stamp of child %d differs from original", i)
		}
	}
}

// TestStampCarrierFormat is spec §8 criterion 1: the parent layout matches
// §3.1 and a full read returns byte-identical content.
func TestStampCarrierFormat(t *testing.T) {
	t.Parallel()

	t.Run("plain full parent", func(t *testing.T) {
		t.Parallel()
		st, _ := newStampingStore(t)
		m, k, c := redundancy.MEDIUM.Composition(false) // 114, 9, 3
		root, data := uploadFile(t, st, m*swarm.ChunkSize, false)
		checkCarriers(t, st, parentPayload(t, st, root), swarm.HashSize, m, k, c)
		if got := readAll(t, context.Background(), st, st, root); !bytes.Equal(got, data) {
			t.Fatal("read-back differs from uploaded data")
		}
	})

	t.Run("plain partial parent", func(t *testing.T) {
		t.Parallel()
		st, _ := newStampingStore(t)
		const m = 30
		k := redundancy.MEDIUM.GetParities(m) // 6
		c := stampcarrier.Count(m + k)        // 1
		root, data := uploadFile(t, st, m*swarm.ChunkSize, false)
		checkCarriers(t, st, parentPayload(t, st, root), swarm.HashSize, m, k, c)
		if got := readAll(t, context.Background(), st, st, root); !bytes.Equal(got, data) {
			t.Fatal("read-back differs from uploaded data")
		}
	})

	t.Run("encrypted full parent", func(t *testing.T) {
		t.Parallel()
		st, _ := newStampingStore(t)
		m, k, c := redundancy.MEDIUM.Composition(true) // 57, 9, 2
		root, data := uploadFile(t, st, m*swarm.ChunkSize, true)
		if len(root.Bytes()) != encryption.ReferenceSize {
			t.Fatal("expected encrypted root reference")
		}
		checkCarriers(t, st, parentPayload(t, st, root), swarm.HashSize+encryption.KeyLength, m, k, c)
		if got := readAll(t, context.Background(), st, st, root); !bytes.Equal(got, data) {
			t.Fatal("read-back differs from uploaded data")
		}
	})

	t.Run("encrypted partial parent", func(t *testing.T) {
		t.Parallel()
		st, _ := newStampingStore(t)
		const m = 20
		k := redundancy.MEDIUM.GetEncParities(m)
		c := stampcarrier.Count(m + k)
		root, data := uploadFile(t, st, m*swarm.ChunkSize, true)
		if len(root.Bytes()) != encryption.ReferenceSize {
			t.Fatal("expected encrypted root reference")
		}
		checkCarriers(t, st, parentPayload(t, st, root), swarm.HashSize+encryption.KeyLength, m, k, c)
		if got := readAll(t, context.Background(), st, st, root); !bytes.Equal(got, data) {
			t.Fatal("read-back differs from uploaded data")
		}
	})

	t.Run("multi level", func(t *testing.T) {
		t.Parallel()
		st, _ := newStampingStore(t)
		m, _, _ := redundancy.MEDIUM.Composition(false)
		// one full level-1 parent plus one carried-over chunk -> two-level trie
		root, data := uploadFile(t, st, (m+1)*swarm.ChunkSize, false)
		// root has 2 children (full parent + elevated chunk), k'=GetParities(2)=3, c'=1
		checkCarriers(t, st, parentPayload(t, st, root), swarm.HashSize, 2, redundancy.MEDIUM.GetParities(2), 1)
		if got := readAll(t, context.Background(), st, st, root); !bytes.Equal(got, data) {
			t.Fatal("read-back differs from uploaded data")
		}
	})
}

// capturePutter records every chunk the decoder saves, keyed by address.
type capturePutter struct {
	store storage.ChunkStore
	mu    sync.Mutex
	saved map[string]swarm.Chunk
}

func newCapturePutter(st storage.ChunkStore) *capturePutter {
	return &capturePutter{store: st, saved: make(map[string]swarm.Chunk)}
}

func (c *capturePutter) Put(ctx context.Context, ch swarm.Chunk) error {
	c.mu.Lock()
	c.saved[ch.Address().ByteString()] = ch
	c.mu.Unlock()
	return c.store.Put(ctx, ch)
}

// gatedStore holds back the retrieval of the listed (healthy) chunks until
// the deleted one has been requested. Without it the RACE strategy can
// conclude - shardCnt shards fetched - before the fetch of the deleted parity
// shard is ever scheduled, in which case that parity is never attempted, never
// rebuilt, and the test flakes. Gating rather than sleeping makes the ordering
// deterministic; the fallback timer only exists so that a regression fails
// instead of hanging.
type gatedStore struct {
	storage.ChunkStore
	victim swarm.Address
	held   map[string]struct{} // addresses held back until victim is requested
	gate   chan struct{}
	once   sync.Once
}

func newGatedStore(st storage.ChunkStore, victim swarm.Address, held []swarm.Address) *gatedStore {
	s := &gatedStore{ChunkStore: st, victim: victim, held: make(map[string]struct{}, len(held)), gate: make(chan struct{})}
	for _, addr := range held {
		if !addr.Equal(victim) {
			s.held[addr.ByteString()] = struct{}{}
		}
	}
	return s
}

func (s *gatedStore) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	if addr.Equal(s.victim) {
		s.once.Do(func() { close(s.gate) })
	} else if _, ok := s.held[addr.ByteString()]; ok {
		select {
		case <-s.gate:
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return s.ChunkStore.Get(ctx, addr)
}

// waitSaved polls until the decoder has saved a chunk for addr (the save runs
// in the prefetch goroutine, i.e. asynchronously to the read).
func waitSaved(t *testing.T, c *capturePutter, addr swarm.Address) swarm.Chunk {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		ch := c.saved[addr.ByteString()]
		c.mu.Unlock()
		if ch != nil {
			return ch
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("chunk %v was not recovered and saved in time", addr)
	return nil
}

// ownerCtx arms the context with strategy fallback and the batch owner
// resolver needed for stamp validation.
func ownerCtx(t *testing.T, owner []byte, strategy getter.Strategy) context.Context {
	t.Helper()
	ctx := getter.SetStrategy(context.Background(), strategy)
	ctx = getter.SetStrict(ctx, false)
	return getter.SetBatchOwnerFn(ctx, func([]byte) ([]byte, error) { return owner, nil })
}

func assertRecoveredStamp(t *testing.T, st *stampingStore, ch swarm.Chunk, owner []byte) {
	t.Helper()
	if ch.Stamp() == nil {
		t.Fatal("recovered chunk has no stamp")
	}
	got, err := postage.NewStamp(ch.Stamp().BatchID(), ch.Stamp().Index(), ch.Stamp().Timestamp(), ch.Stamp().Sig()).MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	want := st.originalStamp(ch.Address())
	if want == nil {
		t.Fatal("no original stamp recorded for the recovered chunk")
	}
	if !bytes.Equal(got, want) {
		t.Fatal("recovered stamp is not byte-identical to the original")
	}
	stamp := new(postage.Stamp)
	if err := stamp.UnmarshalBinary(got); err != nil {
		t.Fatal(err)
	}
	if err := stamp.ValidBinding(ch.Address(), owner); err != nil {
		t.Fatalf("recovered stamp does not validate: %v", err)
	}
}

// refAt returns the 32-byte chunk address of the ref at the given slot of a
// parent payload. Data slots are refLen wide; parity/carrier slots 32.
func refAt(payload []byte, m, refLen, slot int) swarm.Address {
	if slot < m {
		return swarm.NewAddress(payload[slot*refLen : slot*refLen+swarm.HashSize])
	}
	off := m*refLen + (slot-m)*swarm.HashSize
	return swarm.NewAddress(payload[off : off+swarm.HashSize])
}

// TestStampRecoveryData is spec §8 criterion 2: delete a data chunk, RS
// rebuild recovers it together with its original stamp.
func TestStampRecoveryData(t *testing.T) {
	t.Parallel()
	st, owner := newStampingStore(t)
	m, _, _ := redundancy.MEDIUM.Composition(false)
	root, data := uploadFile(t, st, m*swarm.ChunkSize, false)
	payload := parentPayload(t, st, root)

	victim := refAt(payload, m, swarm.HashSize, 5) // a data chunk
	if err := st.Delete(context.Background(), victim); err != nil {
		t.Fatal(err)
	}

	caps := newCapturePutter(st.ChunkStore)
	ctx := ownerCtx(t, owner, getter.DATA) // DATA fails, falls back to RACE
	if got := readAll(t, ctx, st, caps, root); !bytes.Equal(got, data) {
		t.Fatal("read-back differs from uploaded data")
	}
	ch := waitSaved(t, caps, victim)
	assertRecoveredStamp(t, st, ch, owner)
}

// TestStampRecoveryParity is spec §8 criterion 3: a deleted parity chunk is
// rebuilt and its stamp recovered — the case in-scope carrier designs could
// not cover.
func TestStampRecoveryParity(t *testing.T) {
	t.Parallel()
	st, owner := newStampingStore(t)
	m, k, _ := redundancy.MEDIUM.Composition(false)
	root, data := uploadFile(t, st, m*swarm.ChunkSize, false)
	payload := parentPayload(t, st, root)

	victim := refAt(payload, m, swarm.HashSize, m) // first parity chunk
	if err := st.Delete(context.Background(), victim); err != nil {
		t.Fatal(err)
	}

	caps := newCapturePutter(st.ChunkStore)
	ctx := ownerCtx(t, owner, getter.RACE) // RACE attempts parity shards
	// hold the healthy children back so that the missing parity is reliably
	// attempted (and recorded as a genuine miss) before RACE concludes
	children := make([]swarm.Address, 0, m+k)
	for slot := range m + k {
		children = append(children, refAt(payload, m, swarm.HashSize, slot))
	}
	gated := newGatedStore(st, victim, children)
	if got := readAll(t, ctx, gated, caps, root); !bytes.Equal(got, data) {
		t.Fatal("read-back differs from uploaded data")
	}
	ch := waitSaved(t, caps, victim)
	assertRecoveredStamp(t, st, ch, owner)
}

// TestStampRecoveryCarrierLoss is spec §8 criterion 4: with 2 of the carrier
// group members gone, stamps are still recoverable via the carrier RS group.
func TestStampRecoveryCarrierLoss(t *testing.T) {
	t.Parallel()

	t.Run("plain", func(t *testing.T) {
		t.Parallel()
		st, owner := newStampingStore(t)
		m, k, _ := redundancy.MEDIUM.Composition(false) // c=3, group of 5
		root, data := uploadFile(t, st, m*swarm.ChunkSize, false)
		payload := parentPayload(t, st, root)

		// delete one carrier and one carrier parity (2 of 5)
		for _, slot := range []int{m + k, m + k + 3} {
			if err := st.Delete(context.Background(), refAt(payload, m, swarm.HashSize, slot)); err != nil {
				t.Fatal(err)
			}
		}
		victim := refAt(payload, m, swarm.HashSize, 3)
		if err := st.Delete(context.Background(), victim); err != nil {
			t.Fatal(err)
		}

		caps := newCapturePutter(st.ChunkStore)
		ctx := ownerCtx(t, owner, getter.DATA)
		if got := readAll(t, ctx, st, caps, root); !bytes.Equal(got, data) {
			t.Fatal("read-back differs from uploaded data")
		}
		assertRecoveredStamp(t, st, waitSaved(t, caps, victim), owner)
	})

	t.Run("encrypted", func(t *testing.T) {
		t.Parallel()
		st, owner := newStampingStore(t)
		m, k, _ := redundancy.MEDIUM.Composition(true) // c=2, group of 4
		refLen := swarm.HashSize + encryption.KeyLength
		root, data := uploadFile(t, st, m*swarm.ChunkSize, true)
		payload := parentPayload(t, st, root)

		// delete 2 of the 4 carrier-group chunks
		for _, slot := range []int{m + k, m + k + 2} {
			if err := st.Delete(context.Background(), refAt(payload, m, refLen, slot)); err != nil {
				t.Fatal(err)
			}
		}
		victim := refAt(payload, m, refLen, 3)
		if err := st.Delete(context.Background(), victim); err != nil {
			t.Fatal(err)
		}

		caps := newCapturePutter(st.ChunkStore)
		ctx := ownerCtx(t, owner, getter.DATA)
		if got := readAll(t, ctx, st, caps, root); !bytes.Equal(got, data) {
			t.Fatal("read-back differs from uploaded data")
		}
		assertRecoveredStamp(t, st, waitSaved(t, caps, victim), owner)
	})
}

// TestStampRecoveryDegradation is spec §8 criterion 5: with 3 of the 5 group
// members gone the read still succeeds and the rebuilt chunk is saved, just
// without a stamp — never worse than today.
func TestStampRecoveryDegradation(t *testing.T) {
	t.Parallel()
	st, owner := newStampingStore(t)
	m, k, _ := redundancy.MEDIUM.Composition(false)
	root, data := uploadFile(t, st, m*swarm.ChunkSize, false)
	payload := parentPayload(t, st, root)

	for _, slot := range []int{m + k, m + k + 1, m + k + 2} {
		if err := st.Delete(context.Background(), refAt(payload, m, swarm.HashSize, slot)); err != nil {
			t.Fatal(err)
		}
	}
	victim := refAt(payload, m, swarm.HashSize, 7)
	if err := st.Delete(context.Background(), victim); err != nil {
		t.Fatal(err)
	}

	caps := newCapturePutter(st.ChunkStore)
	ctx := ownerCtx(t, owner, getter.DATA)
	if got := readAll(t, ctx, st, caps, root); !bytes.Equal(got, data) {
		t.Fatal("read-back differs from uploaded data")
	}
	ch := waitSaved(t, caps, victim)
	if ch.Stamp() != nil {
		t.Fatal("expected rebuilt chunk to be saved unstamped when the carrier group is unrecoverable")
	}
}

// TestStampRecoveryIntermediate is spec §8 criterion 6: the stamp of an
// intermediate (non-leaf) chunk is recovered via its parent's carriers.
func TestStampRecoveryIntermediate(t *testing.T) {
	t.Parallel()
	st, owner := newStampingStore(t)
	m, _, _ := redundancy.MEDIUM.Composition(false)
	root, data := uploadFile(t, st, (m+1)*swarm.ChunkSize, false) // two-level trie
	payload := parentPayload(t, st, root)

	victim := refAt(payload, 2, swarm.HashSize, 0) // the full level-1 parent chunk
	if err := st.Delete(context.Background(), victim); err != nil {
		t.Fatal(err)
	}

	caps := newCapturePutter(st.ChunkStore)
	ctx := ownerCtx(t, owner, getter.DATA)
	if got := readAll(t, ctx, st, caps, root); !bytes.Equal(got, data) {
		t.Fatal("read-back differs from uploaded data")
	}
	assertRecoveredStamp(t, st, waitSaved(t, caps, victim), owner)
}

// TestStampRecoveryWrongOwner covers the validation branch of the spec §7
// degradation rule: the carrier group is intact and yields an entry, but the
// stamp does not bind to the expected batch owner, so it is discarded and the
// rebuilt chunk is saved unstamped. This is the branch that keeps a wrong or
// tampered carrier from ever producing a forged attribution. The setup is the
// one of TestStampRecoveryData, which shows the very same entry is recovered
// and attached once the resolver returns the real owner - so the difference
// here is validation, not availability.
func TestStampRecoveryWrongOwner(t *testing.T) {
	t.Parallel()
	st, _ := newStampingStore(t)
	m, _, _ := redundancy.MEDIUM.Composition(false)
	root, data := uploadFile(t, st, m*swarm.ChunkSize, false)
	payload := parentPayload(t, st, root)

	victim := refAt(payload, m, swarm.HashSize, 5)
	if err := st.Delete(context.Background(), victim); err != nil {
		t.Fatal(err)
	}

	// resolve the batch to somebody else's address: ValidBinding recovers the
	// real signer from the entry and rejects it with ErrOwnerMismatch
	wrongOwner := make([]byte, 20)
	if _, err := rand.Read(wrongOwner); err != nil {
		t.Fatal(err)
	}

	caps := newCapturePutter(st.ChunkStore)
	ctx := ownerCtx(t, wrongOwner, getter.DATA)
	if got := readAll(t, ctx, st, caps, root); !bytes.Equal(got, data) {
		t.Fatal("read-back differs from uploaded data")
	}
	ch := waitSaved(t, caps, victim)
	if ch.Stamp() != nil {
		t.Fatal("expected rebuilt chunk to be saved unstamped when the recovered stamp does not validate")
	}
}
