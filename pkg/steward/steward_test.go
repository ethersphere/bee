// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package steward_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/postage"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/mock"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/steward"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type counter struct {
	storage.ChunkStore
	count atomic.Int32

	mu       sync.Mutex
	replicas map[string]struct{}
}

func (c *counter) Put(ctx context.Context, ch swarm.Chunk) (err error) {
	c.count.Add(1)
	if isDispersedReplica(ch) {
		c.mu.Lock()
		if c.replicas == nil {
			c.replicas = make(map[string]struct{})
		}
		c.replicas[ch.Address().String()] = struct{}{}
		c.mu.Unlock()
	}
	return c.ChunkStore.Put(ctx, ch)
}

// replicaSet returns a snapshot of the dispersed replica addresses written
// through this store so far.
func (c *counter) replicaSet() map[string]struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]struct{}, len(c.replicas))
	for addr := range c.replicas {
		out[addr] = struct{}{}
	}
	return out
}

// isDispersedReplica reports whether ch is a SOC-wrapped dispersed replica,
// i.e. signed by the well-known replicas owner rather than a user key.
func isDispersedReplica(ch swarm.Chunk) bool {
	sch, err := soc.FromChunk(ch)
	return err == nil && bytes.Equal(sch.OwnerAddress(), swarm.ReplicasOwner)
}

// assertSameReplicas fails if the two address sets differ, reporting what the
// original upload produced versus what the re-upload did.
func assertSameReplicas(t *testing.T, want, got map[string]struct{}) {
	t.Helper()

	if len(got) != len(want) {
		t.Errorf("dispersed replica count: upload produced %d, re-upload produced %d", len(want), len(got))
	}
	for addr := range want {
		if _, ok := got[addr]; !ok {
			t.Errorf("replica %s produced by the upload path but not re-uploaded", addr)
		}
	}
	for addr := range got {
		if _, ok := want[addr]; !ok {
			t.Errorf("replica %s re-uploaded but never produced by the upload path", addr)
		}
	}
}

// recordingStamper wraps a postage.Stamper and records the address each Stamp
// call was made for, so tests can assert every uploaded chunk (including each
// dispersed replica) was stamped against its own address rather than a single
// shared stamp computed once for the root chunk.
type recordingStamper struct {
	postage.Stamper
	mu      sync.Mutex
	stamped map[string]int
}

func newRecordingStamper() *recordingStamper {
	return &recordingStamper{Stamper: postagetesting.NewStamper(), stamped: make(map[string]int)}
}

func (r *recordingStamper) Stamp(addr, idAddr swarm.Address) (*postage.Stamp, error) {
	r.mu.Lock()
	r.stamped[addr.String()]++
	r.mu.Unlock()
	return r.Stamper.Stamp(addr, idAddr)
}

func (r *recordingStamper) stampedFor(addr swarm.Address) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stamped[addr.String()]
}

func TestSteward(t *testing.T) {
	t.Parallel()
	inmem := &counter{ChunkStore: inmemchunkstore.New()}

	var (
		ctx            = context.Background()
		chunks         = 1000
		data           = make([]byte, chunks*4096) // 1k chunks
		chunkStore     = inmem
		store          = mockstorer.NewWithChunkStore(chunkStore)
		localRetrieval = &localRetriever{ChunkStore: chunkStore}
		s              = steward.New(store, localRetrieval, inmem)
		stamper        = postagetesting.NewStamper()
	)
	n, err := rand.Read(data)
	if n != cap(data) {
		t.Fatal("short read")
	}
	if err != nil {
		t.Fatal(err)
	}

	// Upload at the same redundancy level the re-upload uses, so the dispersed
	// replicas the regular upload path creates (hashtrie -> replicas.NewPutter)
	// are actually present to compare the re-uploaded ones against. With
	// redundancy.NONE the upload creates no replicas at all, and the replica
	// assertions below would pass against anything Reupload happened to emit.
	pipe := builder.NewPipelineBuilder(ctx, chunkStore, false, redundancy.PARANOID)
	addr, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	// Snapshot before the re-upload starts writing through the same store.
	uploadReplicas := inmem.replicaSet()
	replicaCount := redundancy.PARANOID.GetReplicaCount()
	if len(uploadReplicas) != replicaCount {
		t.Fatalf("upload path produced %d dispersed replicas, want %d", len(uploadReplicas), replicaCount)
	}

	// Replicas are not part of the trie, so traversal does not walk them:
	// the re-upload pushes the trie chunks plus a fresh set of replicas.
	trieChunkCount := int(inmem.count.Load()) - len(uploadReplicas)
	wantPushed := trieChunkCount + replicaCount
	done := make(chan struct{})
	errc := make(chan error, 1)
	replicaAddrs := make(map[string]struct{})
	var replicaMu sync.Mutex
	go func() {
		defer close(done)
		count := 0
		for op := range store.PusherFeed() {
			// DirectUpload only forwards pushed chunks over the feed; it does not
			// persist them. Persist here so the post-reupload assertions (Has,
			// IsRetrievable) observe pushed-but-not-yet-locally-known chunks the
			// same way a real pushsync round-trip eventually would.
			if err := chunkStore.Put(ctx, op.Chunk); err != nil {
				select {
				case errc <- err:
				default:
				}
				return
			}

			if isDispersedReplica(op.Chunk) {
				replicaMu.Lock()
				replicaAddrs[op.Chunk.Address().String()] = struct{}{}
				replicaMu.Unlock()
			}

			count++
			if count == wantPushed {
				return
			}
		}
	}()

	err = s.Reupload(ctx, addr, stamper, redundancy.PARANOID)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("took too long to finish")
	}

	select {
	case err := <-errc:
		t.Fatalf("unexpected error: %v", err)
	default:
	}

	isRetrievable, err := s.IsRetrievable(ctx, addr, redundancy.PARANOID)
	if err != nil {
		t.Fatal(err)
	}
	if !isRetrievable {
		t.Fatalf("re-uploaded content on %q should be retrievable", addr)
	}

	// IsRetrievable's root-chunk fetch goes through joiner -> replicas.NewGetter,
	// which races the original root address against speculative replica candidate
	// addresses and only cancels the losers once the first fetch wins. Those
	// speculative fetches are for replica addresses, and some of them land before
	// cancellation does, so count only the trie chunks traversal is responsible
	// for and the assertion stays exact instead of needing a tolerance.
	//
	// Read through the accessor: the losing prefetch goroutines can still be
	// writing to the map at this point.
	retrieved := localRetrieval.retrievedSnapshot()
	count := 0
	for addr := range retrieved {
		if _, isReplica := uploadReplicas[addr]; !isReplica {
			count++
		}
	}
	if count != trieChunkCount {
		t.Fatalf("unexpected no of unique non-replica chunks retrieved: want %d have %d", trieChunkCount, count)
	}

	replicaMu.Lock()
	gotReplicas := make(map[string]struct{}, len(replicaAddrs))
	for addr := range replicaAddrs {
		gotReplicas[addr] = struct{}{}
	}
	replicaMu.Unlock()

	// The re-uploaded replicas must be exactly the ones the regular upload path
	// produced: same count and same addresses. Asserting only the count would
	// not catch replicas derived from the wrong root chunk.
	assertSameReplicas(t, uploadReplicas, gotReplicas)
}

// strictAddressChunkStore wraps a storage.ChunkStore and requires Get to be
// called with an exact 32-byte content address - unlike inmemchunkstore, which
// silently truncates longer (e.g. 64-byte encrypted) addresses to the first 32
// bytes on lookup, masking a caller that forgets to trim an encrypted reference
// before deriving replica addresses from it.
type strictAddressChunkStore struct {
	storage.ChunkStore
}

func (s *strictAddressChunkStore) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	if len(addr.Bytes()) != swarm.HashSize {
		return nil, fmt.Errorf("strictAddressChunkStore: Get called with non-content address %s (len %d)", addr, len(addr.Bytes()))
	}
	return s.ChunkStore.Get(ctx, addr)
}

// TestStewardEncryptedReference verifies that Reupload correctly derives dispersed
// replica addresses from an encrypted reference (address + decryption key), by
// trimming it to the 32-byte content address before deriving replicas - otherwise
// the replica addresses computed would not match what a downloader deriving
// replicas from the plain content address expects.
func TestStewardEncryptedReference(t *testing.T) {
	t.Parallel()
	inmem := &counter{ChunkStore: &strictAddressChunkStore{ChunkStore: inmemchunkstore.New()}}

	var (
		ctx        = context.Background()
		chunks     = 3
		data       = make([]byte, chunks*4096)
		chunkStore = inmem
		store      = mockstorer.NewWithChunkStore(chunkStore)
		s          = steward.New(store, &localRetriever{ChunkStore: chunkStore}, inmem)
		stamper    = newRecordingStamper()
	)
	n, err := rand.Read(data)
	if n != cap(data) {
		t.Fatal("short read")
	}
	if err != nil {
		t.Fatal(err)
	}

	// Upload at the same redundancy level as the re-upload, so the replicas the
	// regular upload path derives from the plain content address are present to
	// compare against (see the equivalent comment in TestSteward).
	pipe := builder.NewPipelineBuilder(ctx, chunkStore, true, redundancy.PARANOID)
	addr, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(addr.Bytes()) != swarm.HashSize+32 {
		t.Fatalf("expected an encrypted reference of length %d, got %d", swarm.HashSize+32, len(addr.Bytes()))
	}

	replicaCount := redundancy.PARANOID.GetReplicaCount()
	contentAddr := swarm.NewAddress(addr.Bytes()[:swarm.HashSize])

	// Snapshot before the re-upload writes through the same store.
	uploadReplicas := inmem.replicaSet()
	if len(uploadReplicas) != replicaCount {
		t.Fatalf("upload path produced %d dispersed replicas, want %d", len(uploadReplicas), replicaCount)
	}

	replicaAddrs := make(map[string]struct{})
	var replicaMu sync.Mutex
	done := make(chan struct{})
	errc := make(chan error, 1)
	// Replicas are not walked by traversal, so the re-upload pushes the trie
	// chunks plus a fresh set of replicas.
	wantPushed := int(inmem.count.Load()) - len(uploadReplicas) + replicaCount
	go func() {
		defer close(done)
		count := 0
		for op := range store.PusherFeed() {
			if err := chunkStore.Put(ctx, op.Chunk); err != nil {
				select {
				case errc <- err:
				default:
				}
				return
			}
			if isDispersedReplica(op.Chunk) {
				replicaMu.Lock()
				replicaAddrs[op.Chunk.Address().String()] = struct{}{}
				replicaMu.Unlock()
			}
			count++
			if count == wantPushed {
				return
			}
		}
	}()

	err = s.Reupload(ctx, addr, stamper, redundancy.PARANOID)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("took too long to finish")
	}
	select {
	case err := <-errc:
		t.Fatalf("unexpected error: %v", err)
	default:
	}

	replicaMu.Lock()
	gotReplicas := make(map[string]struct{}, len(replicaAddrs))
	for addr := range replicaAddrs {
		gotReplicas[addr] = struct{}{}
	}
	replicaMu.Unlock()

	// The re-uploaded replicas must be exactly the ones the upload path derived
	// from the plain content address. If Reupload had derived them from the
	// 64-byte encrypted reference instead, the addresses would differ and this
	// would fail even though the count still matched.
	assertSameReplicas(t, uploadReplicas, gotReplicas)

	// Every replica must wrap the plain 32-byte content address's chunk, and
	// replicas.NewPutter derives replica addresses from that same chunk's
	// address (ch.Address()) - so this also proves replica addresses were
	// derived from contentAddr, not the 64-byte encrypted reference. If the
	// reference had not been trimmed before the fix, this lookup would have
	// failed (get root chunk for dispersed replicas) or wrapped the wrong chunk.
	for addrStr := range replicaAddrs {
		replicaAddr := swarm.MustParseHexAddress(addrStr)
		sch, err := chunkStore.Get(ctx, replicaAddr)
		if err != nil {
			t.Fatalf("get replica chunk %s: %v", replicaAddr, err)
		}
		replicaSOC, err := soc.FromChunk(sch)
		if err != nil {
			t.Fatalf("replica %s is not a valid SOC chunk: %v", replicaAddr, err)
		}
		if !replicaSOC.WrappedChunk().Address().Equal(contentAddr) {
			t.Fatalf("replica %s wraps chunk %s, want %s", replicaAddr, replicaSOC.WrappedChunk().Address(), contentAddr)
		}

		// Each replica must be individually stamped against its own SOC
		// address - not stamped once against the root chunk's address and
		// reused, which would fail stamp validation on the receiving side
		// since a postage stamp is only valid for the specific address it
		// was computed against.
		if got := stamper.stampedFor(replicaAddr); got != 1 {
			t.Fatalf("replica %s: want exactly 1 Stamp call for its own address, got %d", replicaAddr, got)
		}
	}
	// The root chunk's own address gets stamped exactly once via the normal
	// traversal path (fn), because it's re-uploaded as part of the trie like any
	// other chunk. It must not be stamped a second time by the replica-upload
	// step: reusing that stamp on a differently-addressed SOC replica chunk
	// would fail stamp validation on the receiving side, since a stamp is only
	// valid for the specific address it was computed against.
	if got := stamper.stampedFor(contentAddr); got != 1 {
		t.Fatalf("root chunk address %s: want exactly 1 Stamp call (from trie traversal), got %d", contentAddr, got)
	}
}

type localRetriever struct {
	storage.ChunkStore
	mu              sync.Mutex
	retrievedChunks map[string]struct{}
}

// retrievedSnapshot returns a copy of the addresses retrieved so far.
//
// The redundancy getter keeps speculative prefetch goroutines running after
// IsRetrievable has returned (they are only cancelled once the first fetch
// wins), so this map is still being written to while the test reads it.
// Callers must go through this accessor rather than touching the map directly.
func (lr *localRetriever) retrievedSnapshot() map[string]struct{} {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	out := make(map[string]struct{}, len(lr.retrievedChunks))
	for addr := range lr.retrievedChunks {
		out[addr] = struct{}{}
	}
	return out
}

func (lr *localRetriever) RetrieveChunk(ctx context.Context, addr, sourceAddr swarm.Address) (chunk swarm.Chunk, err error) {
	ch, err := lr.Get(ctx, addr)
	if err != nil {
		return nil, err
	}

	lr.mu.Lock()
	defer lr.mu.Unlock()

	if lr.retrievedChunks == nil {
		lr.retrievedChunks = make(map[string]struct{})
	}
	lr.retrievedChunks[addr.String()] = struct{}{}
	return ch, nil
}
