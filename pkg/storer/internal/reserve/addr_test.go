// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/storage"
	chunk "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// addrTestElement is one handcrafted address index entry.
type addrTestElement struct {
	addr, stampHash, batchID []byte
}

func (e addrTestElement) key() []byte { return append(slices.Clone(e.addr), e.stampHash...) }

func filled(b byte) []byte { return bytes.Repeat([]byte{b}, swarm.HashSize) }

// addrTestElements returns entries spread over the address space, including
// both ends and addresses shared by several stamp hashes.
func addrTestElements(rnd *rand.Rand) []addrTestElement {
	random := func() []byte {
		b := make([]byte, swarm.HashSize)
		_, _ = rnd.Read(b)
		return b
	}
	addrs := make([][]byte, 0, 66)
	addrs = append(addrs, filled(0x00), filled(0xff), filled(0x7f), filled(0x80))
	low := filled(0x00)
	low[swarm.HashSize-1] = 1
	high := filled(0xff)
	high[swarm.HashSize-1] = 0xfe
	addrs = append(addrs, low, high)
	for range 60 {
		addrs = append(addrs, random())
	}
	var els []addrTestElement
	for i, a := range addrs {
		els = append(els, addrTestElement{addr: a, stampHash: random(), batchID: random()})
		if i%3 == 0 {
			// a second stamp hash, at both ends for the extreme addresses
			sh := random()
			switch i {
			case 0:
				sh = filled(0x00)
			case 1:
				sh = filled(0xff)
			}
			els = append(els, addrTestElement{addr: a, stampHash: sh, batchID: random()})
		}
	}
	return els
}

// putAddrElements writes the entries and a valid marker directly to the
// index store.
func putAddrElements(t *testing.T, st transaction.Storage, els []addrTestElement) {
	t.Helper()

	err := st.Run(context.Background(), func(s transaction.Store) error {
		for _, e := range els {
			err := s.IndexStore().Put(&reserve.ChunkAddrItem{
				Address:   swarm.NewAddress(e.addr),
				StampHash: e.stampHash,
				BatchID:   e.batchID,
				ChunkType: swarm.ChunkTypeContentAddressed,
			})
			if err != nil {
				return err
			}
		}
		return s.IndexStore().Put(&reserve.ChunkAddrValidItem{Version: reserve.ChunkAddrIndexVersion})
	})
	if err != nil {
		t.Fatal(err)
	}
}

// hasBitPrefix is the brute-force reference: the first bits bits of addr
// equal those of prefix, missing prefix bytes counting as zero.
func hasBitPrefix(addr, prefix []byte, bits int) bool {
	for i := range bits {
		var p byte
		if i/8 < len(prefix) {
			p = prefix[i/8]
		}
		if (addr[i/8]>>(7-i%8))&1 != (p>>(7-i%8))&1 {
			return false
		}
	}
	return true
}

func expectedIDs(els []addrTestElement, prefix []byte, bits int, after []byte) [][]byte {
	var want [][]byte
	for _, e := range els {
		k := e.key()
		if !hasBitPrefix(e.addr, prefix, bits) {
			continue
		}
		if after != nil && bytes.Compare(k, after) <= 0 {
			continue
		}
		want = append(want, k)
	}
	slices.SortFunc(want, bytes.Compare)
	return want
}

type addrStore struct {
	name string
	st   transaction.Storage
}

func addrStores(t *testing.T) []addrStore {
	t.Helper()
	return []addrStore{
		{"leveldb", newReserveDiskStorage(t)},
		{"inmem", internal.NewInmemStorage()},
	}
}

// TestIterateAddr checks IterateAddr against brute force for prefixes of
// several bit lengths, including bits 0 and ranges at the top of the address
// space (which reach the end of the namespace), and every kind of cursor, on
// both the leveldb and the in-memory store, with the validity marker present.
func TestIterateAddr(t *testing.T) {
	t.Parallel()

	rnd := rand.New(rand.NewSource(1))
	els := addrTestElements(rnd)
	batchOf := make(map[string][]byte)
	for _, e := range els {
		batchOf[string(e.key())] = e.batchID
	}

	for _, s := range addrStores(t) {
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()

			putAddrElements(t, s.st, els)

			sorted := expectedIDs(els, nil, 0, nil)
			if len(sorted) != len(els) {
				t.Fatalf("%d elements, %d distinct", len(els), len(sorted))
			}

			prefixes := [][]byte{filled(0x00), filled(0xff), filled(0x7f), filled(0x80)}
			for i := 0; i < len(els); i += 7 {
				prefixes = append(prefixes, els[i].addr)
			}
			random := make([]byte, swarm.HashSize)
			_, _ = rnd.Read(random)
			prefixes = append(prefixes, random)

			for _, bits := range []int{0, 1, 4, 8, 12, 13, 255} {
				for _, p := range prefixes {
					// the full address (bits past bits set), the canonical
					// ceil(bits/8) prefix and a prefix shorter than that
					masked := slices.Clone(p[:(bits+7)/8])
					if bits%8 != 0 {
						masked[len(masked)-1] &= 0xff << (8 - bits%8)
					}
					forms := [][]byte{p, masked}
					if len(masked) > 1 {
						forms = append(forms, masked[:len(masked)-1])
					}
					for _, prefix := range forms {
						want := expectedIDs(els, prefix, bits, nil)
						afters := [][]byte{nil, {0x00}, bytes.Repeat([]byte{0xff}, 2*swarm.HashSize), p}
						if len(want) > 0 {
							afters = append(afters,
								want[0],
								want[len(want)-1],
								append(slices.Clone(want[0]), 0x00),
								want[0][:swarm.HashSize],
							)
							if len(want) > 2 {
								afters = append(afters, want[len(want)/2])
							}
						}
						for _, after := range afters {
							checkIterateAddr(t, s.st, els, batchOf, prefix, bits, after)
						}
					}
				}
			}
			// the empty prefix at bits 0
			checkIterateAddr(t, s.st, els, batchOf, nil, 0, nil)
			checkIterateAddr(t, s.st, els, batchOf, nil, 0, sorted[len(sorted)-1])
		})
	}
}

func checkIterateAddr(t *testing.T, st transaction.Storage, els []addrTestElement, batchOf map[string][]byte, prefix []byte, bits int, after []byte) {
	t.Helper()

	want := expectedIDs(els, prefix, bits, after)
	var got [][]byte
	err := reserve.IterateAddr(st.IndexStore(), swarm.NewAddress(prefix), uint8(bits), after, func(c reserve.ChunkID) (bool, error) {
		if len(c.Address.Bytes()) != swarm.HashSize || len(c.StampHash) != swarm.HashSize {
			return true, fmt.Errorf("element of %d+%d bytes", len(c.Address.Bytes()), len(c.StampHash))
		}
		k := append(slices.Clone(c.Address.Bytes()), c.StampHash...)
		if !bytes.Equal(c.BatchID, batchOf[string(k)]) {
			return true, fmt.Errorf("batch ID %x of %x, want %x", c.BatchID, k, batchOf[string(k)])
		}
		got = append(got, k)
		return false, nil
	})
	if err != nil {
		t.Fatalf("prefix %x/%d after %x: %v", prefix, bits, after, err)
	}
	if !slices.EqualFunc(got, want, bytes.Equal) {
		t.Fatalf("prefix %x/%d after %x: got %d ids, want %d", prefix, bits, after, len(got), len(want))
	}
}

func TestIterateAddrStopAndError(t *testing.T) {
	t.Parallel()

	els := addrTestElements(rand.New(rand.NewSource(2)))
	for _, s := range addrStores(t) {
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()

			putAddrElements(t, s.st, els)
			all := expectedIDs(els, nil, 0, nil)

			var got [][]byte
			err := reserve.IterateAddr(s.st.IndexStore(), swarm.ZeroAddress, 0, nil, func(c reserve.ChunkID) (bool, error) {
				got = append(got, append(slices.Clone(c.Address.Bytes()), c.StampHash...))
				return len(got) == 5, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.EqualFunc(got, all[:5], bytes.Equal) {
				t.Fatal("stop: wrong elements")
			}

			errStop := errors.New("stop here")
			err = reserve.IterateAddr(s.st.IndexStore(), swarm.ZeroAddress, 0, nil, func(reserve.ChunkID) (bool, error) {
				return false, errStop
			})
			if !errors.Is(err, errStop) {
				t.Fatalf("callback error: %v", err)
			}
		})
	}
}

// TestAddrIndexElementID checks which iteration result IDs are accepted: 64
// bytes (inmemstore) or "/" + 64 bytes (leveldbstore with PrefixAtStart).
func TestAddrIndexElementID(t *testing.T) {
	t.Parallel()

	id := string(append(filled(1), filled(2)...))
	for _, tc := range []struct {
		in   string
		want string
		ok   bool
	}{
		{id, id, true},
		{"/" + id, id, true},
		{"x" + id, "", false},
		{id[1:], "", false},
		{id + "/", "", false},
		{"//" + id, "", false},
		{"", "", false},
	} {
		got, err := reserve.AddrIndexElementID(tc.in)
		if tc.ok != (err == nil) || got != tc.want {
			t.Fatalf("id of %d bytes: %q, %v", len(tc.in), got, err)
		}
	}
}

// TestIterateAddrSnapshot checks on leveldb that one IterateAddr call reads
// one snapshot: commits made after the first callback are not seen.
func TestIterateAddrSnapshot(t *testing.T) {
	t.Parallel()

	st := newReserveDiskStorage(t)
	els := addrTestElements(rand.New(rand.NewSource(3)))
	putAddrElements(t, st, els)
	all := expectedIDs(els, nil, 0, nil)

	var got [][]byte
	err := reserve.IterateAddr(st.IndexStore(), swarm.ZeroAddress, 0, nil, func(c reserve.ChunkID) (bool, error) {
		if len(got) == 0 {
			// delete an element ahead of the cursor, insert one, overwrite one
			err := st.Run(context.Background(), func(s transaction.Store) error {
				last := all[len(all)-1]
				mid := all[len(all)/2]
				extra := append(filled(0x90), filled(0x01)...)
				return errors.Join(
					s.IndexStore().Delete(&reserve.ChunkAddrItem{Address: swarm.NewAddress(last[:32]), StampHash: last[32:]}),
					s.IndexStore().Put(&reserve.ChunkAddrItem{Address: swarm.NewAddress(extra[:32]), StampHash: extra[32:], BatchID: filled(9)}),
					s.IndexStore().Put(&reserve.ChunkAddrItem{Address: swarm.NewAddress(mid[:32]), StampHash: mid[32:], BatchID: filled(9)}),
				)
			})
			if err != nil {
				return true, err
			}
		}
		k := append(slices.Clone(c.Address.Bytes()), c.StampHash...)
		if bytes.Equal(c.BatchID, filled(9)) {
			return true, fmt.Errorf("saw a write committed after the iteration opened: %x", k)
		}
		got = append(got, k)
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.EqualFunc(got, all, bytes.Equal) {
		t.Fatalf("got %d ids, want the %d of the snapshot", len(got), len(all))
	}
}

// TestIterateAddrNoReserveLock checks that the iteration opens and calls its
// first callback while a writer holds the batch lock and the registration
// read lock and waits inside the observer's Commit (the T1.6 rebuild relies
// on it, see storer.ReserveAddrStore).
func TestIterateAddrNoReserveLock(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)
	st := newReserveDiskStorage(t)
	r := newAddrIndexReserve(t, baseAddr, st, true)

	batch := postagetesting.MustNewBatch()
	first := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewBatchStamp(batch.ID))
	if err := r.Put(ctx, first); err != nil {
		t.Fatal(err)
	}

	o := &blockingObserver{entered: make(chan struct{}), release: make(chan struct{})}
	if err := r.SetObserver(o); err != nil {
		t.Fatal(err)
	}
	putDone := make(chan error, 1)
	go func() {
		ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewBatchStamp(batch.ID))
		putDone <- r.Put(ctx, ch)
	}()
	<-o.entered // the Put holds the batch lock and the registration read lock

	iterDone := make(chan error, 1)
	var n atomic.Int32
	go func() {
		iterDone <- r.IterateAddr(swarm.ZeroAddress, 0, nil, func(reserve.ChunkID) (bool, error) {
			n.Add(1)
			return false, nil
		})
	}()
	select {
	case err := <-iterDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("IterateAddr blocked behind a writer holding the batch lock")
	}
	if n.Load() != 1 {
		t.Fatalf("iterated %d elements, want the committed one", n.Load())
	}
	close(o.release)
	if err := <-putDone; err != nil {
		t.Fatal(err)
	}
}

func TestAddrIndexGetAndHasByID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)
	st := newReserveDiskStorage(t)
	r := newAddrIndexReserve(t, baseAddr, st, true)

	batch := postagetesting.MustNewBatch()
	other := postagetesting.MustNewBatch()
	ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, 1).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 1))
	keep := chunk.GenerateTestRandomChunkAt(t, baseAddr, 2).WithStamp(postagetesting.MustNewBatchStamp(other.ID))
	for _, c := range []swarm.Chunk{ch, keep} {
		if err := r.Put(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	id := chunkIDOf(t, ch)

	got, err := r.GetByID(ctx, id.Address, id.StampHash)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(ch) || !bytes.Equal(got.Stamp().BatchID(), batch.ID) {
		t.Fatal("GetByID returned another chunk or stamp")
	}
	gotHash, err := got.Stamp().Hash()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotHash, id.StampHash) {
		t.Fatal("GetByID returned another stamp")
	}
	if has, err := r.HasID(id.Address, id.StampHash); err != nil || !has {
		t.Fatalf("HasID: %v, %v", has, err)
	}

	unknown := swarm.RandAddress(t)
	if _, err := r.GetByID(ctx, unknown, id.StampHash); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("GetByID of an unknown id: %v", err)
	}
	if has, err := r.HasID(id.Address, filled(3)); err != nil || has {
		t.Fatalf("HasID of an unknown stamp hash: %v, %v", has, err)
	}

	// same address, newer stamp: the old id is gone, the new one is there
	newer := swarm.NewChunk(ch.Address(), ch.Data()).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 2))
	if err := r.Put(ctx, newer); err != nil {
		t.Fatal(err)
	}
	newID := chunkIDOf(t, newer)
	if has, err := r.HasID(id.Address, id.StampHash); err != nil || has {
		t.Fatalf("HasID of the replaced id: %v, %v", has, err)
	}
	if has, err := r.HasID(newID.Address, newID.StampHash); err != nil || !has {
		t.Fatalf("HasID of the newer id: %v, %v", has, err)
	}
	if _, err := r.GetByID(ctx, id.Address, id.StampHash); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("GetByID of the replaced id: %v", err)
	}

	// after eviction
	if _, err := r.EvictBatchBin(ctx, batch.ID, 10, swarm.MaxBins); err != nil {
		t.Fatal(err)
	}
	if has, err := r.HasID(newID.Address, newID.StampHash); err != nil || has {
		t.Fatalf("HasID after eviction: %v, %v", has, err)
	}
	if _, err := r.GetByID(ctx, newID.Address, newID.StampHash); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("GetByID after eviction: %v", err)
	}
	keepID := chunkIDOf(t, keep)
	if got, err := r.GetByID(ctx, keepID.Address, keepID.StampHash); err != nil || !got.Equal(keep) {
		t.Fatalf("GetByID of the other batch's chunk: %v", err)
	}

	// disabled
	off := newAddrIndexReserve(t, baseAddr, st, false)
	if _, err := off.GetByID(ctx, keepID.Address, keepID.StampHash); !errors.Is(err, reserve.ErrAddrIndexDisabled) {
		t.Fatalf("GetByID disabled: %v", err)
	}
	if _, err := off.HasID(keepID.Address, keepID.StampHash); !errors.Is(err, reserve.ErrAddrIndexDisabled) {
		t.Fatalf("HasID disabled: %v", err)
	}
	if err := off.IterateAddr(swarm.ZeroAddress, 0, nil, func(reserve.ChunkID) (bool, error) { return false, nil }); !errors.Is(err, reserve.ErrAddrIndexDisabled) {
		t.Fatalf("IterateAddr disabled: %v", err)
	}
	if off.AddrIndexReady() {
		t.Fatal("disabled index reports ready")
	}
}

// populateReserve puts chunks over several bins and batches with the
// address index disabled, then evicts one batch, half of it pinned, so the
// store also holds pinned chunks without reserve entries.
func populateReserve(t *testing.T, baseAddr swarm.Address, st transaction.Storage, n int) {
	t.Helper()

	ctx := context.Background()
	r := newAddrIndexReserve(t, baseAddr, st, false)
	batchIDs := [][]byte{postagetesting.MustNewBatch().ID, postagetesting.MustNewBatch().ID, postagetesting.MustNewBatch().ID}
	var evictable []swarm.Chunk
	for i := range n {
		ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, i%5).WithStamp(postagetesting.MustNewBatchStamp(batchIDs[i%3]))
		if err := r.Put(ctx, ch); err != nil {
			t.Fatal(err)
		}
		if i%3 == 2 {
			evictable = append(evictable, ch)
		}
	}
	pinChunks(t, st, evictable[:len(evictable)/2]...)
	if _, err := r.EvictBatchBin(ctx, batchIDs[2], len(evictable), swarm.MaxBins); err != nil {
		t.Fatal(err)
	}
}

func readDigest(t *testing.T, st transaction.Storage) reserve.AddrIndexDigest {
	t.Helper()

	d, err := reserve.ReadAddrIndexDigest(context.Background(), st.IndexStore())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func loadMarker(t *testing.T, st transaction.Storage) (*reserve.ChunkAddrValidItem, bool) {
	t.Helper()

	m := &reserve.ChunkAddrValidItem{}
	err := st.IndexStore().Get(m)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return nil, false
	case err != nil:
		t.Fatal(err)
	}
	return m, true
}

// TestAddrIndexFill checks the start-up path of an enabled index without a
// valid marker: the check drops stale entries, the fill writes exactly the
// ChunkBinItem set, the marker carries the reserve's digest, and the index
// reports ready only at the end.
func TestAddrIndexFill(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	for _, batchSize := range []int{1, 7, 10_000} {
		t.Run(fmt.Sprintf("batch=%d", batchSize), func(t *testing.T) {
			t.Parallel()

			baseAddr := swarm.RandAddress(t)
			st := newReserveDiskStorage(t)
			populateReserve(t, baseAddr, st, 30)
			// stale leftovers of an earlier index
			stale := addrTestElements(rand.New(rand.NewSource(4)))[:5]
			putAddrElements(t, st, stale)
			deleteMarker(t, st)

			r := newAddrIndexReserve(t, baseAddr, st, true, reserve.WithAddrIndexBatchSize(batchSize))
			if r.AddrIndexReady() {
				t.Fatal("ready before the check")
			}
			needsFill, dropped, err := r.CheckAddrIndex(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !needsFill || dropped != len(stale) {
				t.Fatalf("check: needsFill %v, dropped %d, want true, %d", needsFill, dropped, len(stale))
			}
			if n := countAddrIndex(t, st); n != 0 {
				t.Fatalf("%d entries left after the drop", n)
			}
			if r.AddrIndexReady() {
				t.Fatal("ready before the fill")
			}
			filled, err := r.FillAddrIndex(ctx)
			if err != nil {
				t.Fatal(err)
			}
			set := checkAddrIndexMatchesReserve(t, st)
			if filled != len(set) || len(set) == 0 {
				t.Fatalf("filled %d, index has %d", filled, len(set))
			}
			if !r.AddrIndexReady() {
				t.Fatal("not ready after the fill")
			}
			m, ok := loadMarker(t, st)
			if !ok || m.Version != reserve.ChunkAddrIndexVersion || m.Digest != readDigest(t, st) {
				t.Fatalf("marker %+v after the fill", m)
			}

			// a second reserve on the same store finds the marker valid
			r2 := newAddrIndexReserve(t, baseAddr, st, true)
			needsFill, dropped, err = r2.CheckAddrIndex(ctx)
			if err != nil || needsFill || dropped != 0 || !r2.AddrIndexReady() {
				t.Fatalf("valid marker: needsFill %v dropped %d ready %v: %v", needsFill, dropped, r2.AddrIndexReady(), err)
			}
		})
	}
}

func deleteMarker(t *testing.T, st transaction.Storage) {
	t.Helper()

	err := st.Run(context.Background(), func(s transaction.Store) error {
		return s.IndexStore().Delete(&reserve.ChunkAddrValidItem{})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func putMarker(t *testing.T, st transaction.Storage, m *reserve.ChunkAddrValidItem) {
	t.Helper()

	err := st.Run(context.Background(), func(s transaction.Store) error { return s.IndexStore().Put(m) })
	if err != nil {
		t.Fatal(err)
	}
}

// cancelingStorage cancels its context after the given number of committed
// transactions, to interrupt a fill.
type cancelingStorage struct {
	transaction.Storage
	after  int32
	n      atomic.Int32
	cancel context.CancelFunc
}

func (c *cancelingStorage) Run(ctx context.Context, f func(transaction.Store) error) error {
	err := c.Storage.Run(ctx, f)
	if c.n.Add(1) == c.after {
		c.cancel()
	}
	return err
}

// TestAddrIndexFillCanceled checks that an interrupted fill leaves no marker,
// and that the next start drops the partial index and refills it exactly.
func TestAddrIndexFillCanceled(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)
	st := newReserveDiskStorage(t)
	populateReserve(t, baseAddr, st, 30)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cst := &cancelingStorage{Storage: st, after: 2, cancel: cancel}
	r := newAddrIndexReserve(t, baseAddr, cst, true, reserve.WithAddrIndexBatchSize(3))
	needsFill, _, err := r.CheckAddrIndex(ctx)
	if err != nil || !needsFill {
		t.Fatalf("check: %v, %v", needsFill, err)
	}
	cst.n.Store(0)
	if _, err := r.FillAddrIndex(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted fill: %v", err)
	}
	if r.AddrIndexReady() {
		t.Fatal("ready after an interrupted fill")
	}
	if _, ok := loadMarker(t, st); ok {
		t.Fatal("marker written by an interrupted fill")
	}
	partial := countAddrIndex(t, st)
	if partial == 0 {
		t.Fatal("the fill was interrupted before writing anything; the test does not cover a partial index")
	}

	r2 := newAddrIndexReserve(t, baseAddr, st, true)
	needsFill, dropped, err := r2.CheckAddrIndex(context.Background())
	if err != nil || !needsFill || dropped != partial {
		t.Fatalf("next start: needsFill %v dropped %d (partial %d): %v", needsFill, dropped, partial, err)
	}
	if _, err := r2.FillAddrIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	checkAddrIndexMatchesReserve(t, st)
}

// TestAddrIndexCheck checks when a marker is trusted: only with the current
// version and the digest of the reserve as it is.
func TestAddrIndexCheck(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// fresh returns a reserve whose index was filled and marked.
	fresh := func(t *testing.T) (swarm.Address, transaction.Storage) {
		t.Helper()
		baseAddr := swarm.RandAddress(t)
		st := newReserveDiskStorage(t)
		populateReserve(t, baseAddr, st, 12)
		r := newAddrIndexReserve(t, baseAddr, st, true)
		if _, _, err := r.CheckAddrIndex(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := r.FillAddrIndex(ctx); err != nil {
			t.Fatal(err)
		}
		return baseAddr, st
	}
	expectRefill := func(t *testing.T, baseAddr swarm.Address, st transaction.Storage) {
		t.Helper()
		r := newAddrIndexReserve(t, baseAddr, st, true)
		needsFill, _, err := r.CheckAddrIndex(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !needsFill {
			t.Fatal("marker trusted for a changed reserve")
		}
		if _, ok := loadMarker(t, st); ok {
			t.Fatal("untrusted marker not deleted")
		}
		if _, err := r.FillAddrIndex(ctx); err != nil {
			t.Fatal(err)
		}
		checkAddrIndexMatchesReserve(t, st)
	}

	t.Run("unchanged", func(t *testing.T) {
		t.Parallel()
		baseAddr, st := fresh(t)
		r := newAddrIndexReserve(t, baseAddr, st, true)
		needsFill, dropped, err := r.CheckAddrIndex(ctx)
		if err != nil || needsFill || dropped != 0 {
			t.Fatalf("needsFill %v dropped %d: %v", needsFill, dropped, err)
		}
	})
	t.Run("other version", func(t *testing.T) {
		t.Parallel()
		baseAddr, st := fresh(t)
		m, _ := loadMarker(t, st)
		putMarker(t, st, &reserve.ChunkAddrValidItem{Version: reserve.ChunkAddrIndexVersion + 1, Digest: m.Digest})
		expectRefill(t, baseAddr, st)
	})
	t.Run("put while disabled", func(t *testing.T) {
		t.Parallel()
		baseAddr, st := fresh(t)
		off := newAddrIndexReserve(t, baseAddr, st, false)
		if err := off.Put(ctx, chunk.GenerateTestRandomChunkAt(t, baseAddr, 3)); err != nil {
			t.Fatal(err)
		}
		expectRefill(t, baseAddr, st)
	})
	t.Run("evict while disabled", func(t *testing.T) {
		t.Parallel()
		baseAddr, st := fresh(t)
		off := newAddrIndexReserve(t, baseAddr, st, false)
		var batchID []byte
		err := st.IndexStore().Iterate(storage.Query{Factory: func() storage.Item { return &reserve.ChunkBinItem{} }}, func(res storage.Result) (bool, error) {
			batchID = res.Entry.(*reserve.ChunkBinItem).BatchID
			return true, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if n, err := off.EvictBatchBin(ctx, batchID, 1, swarm.MaxBins); err != nil || n != 1 {
			t.Fatalf("evicted %d: %v", n, err)
		}
		expectRefill(t, baseAddr, st)
	})
	t.Run("reset while disabled", func(t *testing.T) {
		t.Parallel()
		baseAddr, st := fresh(t)
		off := newAddrIndexReserve(t, baseAddr, st, false)
		if err := off.Reset(ctx); err != nil {
			t.Fatal(err)
		}
		// the next start recreates the epoch
		expectRefill(t, baseAddr, st)
	})
	t.Run("entries without marker", func(t *testing.T) {
		t.Parallel()
		baseAddr, st := fresh(t)
		deleteMarker(t, st)
		expectRefill(t, baseAddr, st)
	})
}

// digestHookReader runs hook before the first iteration of BatchRadiusItems,
// i.e. between the digest's bin ID reads and its count.
type digestHookReader struct {
	storage.Reader
	hook func()
	done atomic.Bool
}

func (d *digestHookReader) Iterate(q storage.Query, fn storage.IterateFn) error {
	if q.Factory().Namespace() == (&reserve.BatchRadiusItem{}).Namespace() && d.done.CompareAndSwap(false, true) {
		d.hook()
	}
	return d.Reader.Iterate(q, fn)
}

func TestReadAddrIndexDigest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)
	st := newReserveDiskStorage(t)

	// an empty store: no epoch, no bins, no items
	if d := readDigest(t, st); d != (reserve.AddrIndexDigest{}) {
		t.Fatalf("digest of an empty store %+v", d)
	}

	r := newAddrIndexReserve(t, baseAddr, st, true)
	chunks := make([]swarm.Chunk, 0, 6)
	for i := range cap(chunks) {
		ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, i%3)
		if err := r.Put(ctx, ch); err != nil {
			t.Fatal(err)
		}
		chunks = append(chunks, ch)
	}
	ids, epoch, err := r.LastBinIDs()
	if err != nil {
		t.Fatal(err)
	}
	d := readDigest(t, st)
	if d.Epoch != epoch || d.Epoch == 0 || d.Count != uint64(len(chunks)) || !slices.Equal(d.BinIDs[:], ids) {
		t.Fatalf("digest %+v, epoch %d bin IDs %v", d, epoch, ids)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := reserve.ReadAddrIndexDigest(canceled, st.IndexStore()); !errors.Is(err, context.Canceled) {
		t.Fatalf("digest with a canceled context: %v", err)
	}

	// A put landing between the bin ID reads and the count: the recorded bin
	// IDs stay behind the store's, so no later state matches, not even after
	// a removal brings the count back.
	torn, err := reserve.ReadAddrIndexDigest(ctx, &digestHookReader{Reader: st.IndexStore(), hook: func() {
		if err := r.Put(ctx, chunk.GenerateTestRandomChunkAt(t, baseAddr, 0)); err != nil {
			t.Error(err)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if torn.Count != uint64(len(chunks))+1 || torn.BinIDs != d.BinIDs {
		t.Fatalf("torn digest %+v, want the old bin IDs and the new count", torn)
	}
	if readDigest(t, st) == torn {
		t.Fatal("torn digest matches the store")
	}
	if _, err := r.EvictBatchBin(ctx, chunks[0].Stamp().BatchID(), 1, swarm.MaxBins); err != nil {
		t.Fatal(err)
	}
	if now := readDigest(t, st); now == torn || now.Count != uint64(len(chunks)) {
		t.Fatalf("digest after a removal %+v matches the torn one", now)
	}

	// after a reset the epoch is absent
	if err := r.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	if d := readDigest(t, st); d.Epoch != 0 || d.Count != 0 {
		t.Fatalf("digest after a reset %+v", d)
	}
}

// writeCountingStorage counts index store writes (puts and deletes).
type writeCountingStorage struct {
	transaction.Storage
	writes atomic.Int32
}

type writeCountingStore struct {
	transaction.Store
	writes *atomic.Int32
}

type writeCountingIndexStore struct {
	storage.IndexStore
	writes *atomic.Int32
}

func (w *writeCountingStorage) Run(ctx context.Context, f func(transaction.Store) error) error {
	return w.Storage.Run(ctx, func(s transaction.Store) error {
		return f(&writeCountingStore{Store: s, writes: &w.writes})
	})
}

func (w *writeCountingStore) IndexStore() storage.IndexStore {
	return &writeCountingIndexStore{IndexStore: w.Store.IndexStore(), writes: w.writes}
}

func (w *writeCountingIndexStore) Put(i storage.Item) error {
	w.writes.Add(1)
	return w.IndexStore.Put(i)
}

func (w *writeCountingIndexStore) Delete(i storage.Item) error {
	w.writes.Add(1)
	return w.IndexStore.Delete(i)
}

// TestDisableAddrIndex checks the disabled start-up path: the marker is
// deleted only if present (a delete of an absent key still writes a leveldb
// tombstone), and leftover entries are reported for the background drop.
func TestDisableAddrIndex(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("nothing there", func(t *testing.T) {
		t.Parallel()
		st := &writeCountingStorage{Storage: newReserveDiskStorage(t)}
		populateReserve(t, swarm.RandAddress(t), st.Storage, 6)
		leftovers, err := reserve.DisableAddrIndex(ctx, st)
		if err != nil || leftovers {
			t.Fatalf("leftovers %v: %v", leftovers, err)
		}
		if n := st.writes.Load(); n != 0 {
			t.Fatalf("%d writes on a store without an address index", n)
		}
	})
	t.Run("marker and entries", func(t *testing.T) {
		t.Parallel()
		inner := newReserveDiskStorage(t)
		st := &writeCountingStorage{Storage: inner}
		putAddrElements(t, inner, addrTestElements(rand.New(rand.NewSource(5)))[:4])
		leftovers, err := reserve.DisableAddrIndex(ctx, st)
		if err != nil || !leftovers {
			t.Fatalf("leftovers %v: %v", leftovers, err)
		}
		if _, ok := loadMarker(t, inner); ok {
			t.Fatal("marker not deleted")
		}
		if n := st.writes.Load(); n != 1 {
			t.Fatalf("%d writes, want the marker delete only", n)
		}
	})
}

func TestDropAddrIndex(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	for _, batch := range []int{1, 3, 10_000} {
		t.Run(fmt.Sprintf("batch=%d", batch), func(t *testing.T) {
			t.Parallel()
			st := newReserveDiskStorage(t)
			els := addrTestElements(rand.New(rand.NewSource(6)))
			putAddrElements(t, st, els)
			n, err := reserve.DropAddrIndexBatch(ctx, st, batch)
			if err != nil || n != len(els) {
				t.Fatalf("dropped %d of %d: %v", n, len(els), err)
			}
			if c := countAddrIndex(t, st); c != 0 {
				t.Fatalf("%d entries left", c)
			}
			// the marker is not part of the namespace
			if _, ok := loadMarker(t, st); !ok {
				t.Fatal("drop removed the marker")
			}
		})
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	st := newReserveDiskStorage(t)
	putAddrElements(t, st, addrTestElements(rand.New(rand.NewSource(7))))
	if _, err := reserve.DropAddrIndex(canceled, st); !errors.Is(err, context.Canceled) {
		t.Fatalf("drop with a canceled context: %v", err)
	}
}

// TestWriteAddrIndexMarker checks the marker rewrite used by Close and after
// a reset: it records the current digest and makes the index ready.
func TestWriteAddrIndexMarker(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)
	st := newReserveDiskStorage(t)
	r := newAddrIndexReserve(t, baseAddr, st, true)
	if _, _, err := r.CheckAddrIndex(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := r.FillAddrIndex(ctx); err != nil {
		t.Fatal(err)
	}
	if err := r.Put(ctx, chunk.GenerateTestRandomChunkAt(t, baseAddr, 0)); err != nil {
		t.Fatal(err)
	}
	if m, _ := loadMarker(t, st); m.Digest == readDigest(t, st) {
		t.Fatal("marker still matches after a put")
	}
	if err := r.WriteAddrIndexMarker(ctx); err != nil {
		t.Fatal(err)
	}
	if m, _ := loadMarker(t, st); m.Digest != readDigest(t, st) {
		t.Fatal("rewritten marker does not match")
	}

	off := newAddrIndexReserve(t, baseAddr, st, false)
	if err := off.WriteAddrIndexMarker(ctx); !errors.Is(err, reserve.ErrAddrIndexDisabled) {
		t.Fatalf("marker write with the index disabled: %v", err)
	}
}

// TestAddrIndexNamespaces checks that no storer namespace has the
// ChunkAddrItem namespace as a byte prefix (leveldb PrefixAtStart iterations
// of the index would reach it).
func TestAddrIndexNamespaces(t *testing.T) {
	t.Parallel()

	ns := (&reserve.ChunkAddrItem{}).Namespace()
	for _, item := range []storage.Item{
		&reserve.ChunkAddrValidItem{}, &reserve.BatchRadiusItem{}, &reserve.ChunkBinItem{},
		&reserve.BinItem{}, &reserve.EpochItem{}, &reserve.RadiusItem{},
	} {
		if strings.HasPrefix(item.Namespace(), ns) {
			t.Fatalf("namespace %q starts with %q", item.Namespace(), ns)
		}
	}
}
