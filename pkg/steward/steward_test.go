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

	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/manifest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/mock"
	"github.com/ethersphere/bee/v2/pkg/pusher"
	"github.com/ethersphere/bee/v2/pkg/soc"
	testingsoc "github.com/ethersphere/bee/v2/pkg/soc/testing"
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

// recordingStamper wraps a postage.Stamper and records the address and
// identity address each Stamp call was made for.
type recordingStamper struct {
	postage.Stamper
	mu      sync.Mutex
	stamped map[string]int
	idAddrs map[string]swarm.Address
}

func newRecordingStamper() *recordingStamper {
	return &recordingStamper{
		Stamper: postagetesting.NewStamper(),
		stamped: make(map[string]int),
		idAddrs: make(map[string]swarm.Address),
	}
}

func (r *recordingStamper) Stamp(addr, idAddr swarm.Address) (*postage.Stamp, error) {
	r.mu.Lock()
	r.stamped[addr.String()]++
	r.idAddrs[addr.String()] = idAddr
	r.mu.Unlock()
	return r.Stamper.Stamp(addr, idAddr)
}

func (r *recordingStamper) stampedFor(addr swarm.Address) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stamped[addr.String()]
}

func (r *recordingStamper) idAddrFor(addr swarm.Address) swarm.Address {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.idAddrs[addr.String()]
}

// assertReplicaStamps checks every replica was stamped once, keyed by its
// identity address, as the upload path does.
func assertReplicaStamps(t *testing.T, ctx context.Context, cs storage.ChunkStore, stamper *recordingStamper, replicaAddrs map[string]struct{}) {
	t.Helper()

	for addrStr := range replicaAddrs {
		addr := swarm.MustParseHexAddress(addrStr)
		ch, err := cs.Get(ctx, addr)
		if err != nil {
			t.Fatalf("get replica %s: %v", addr, err)
		}
		if got := stamper.stampedFor(addr); got != 1 {
			t.Fatalf("replica %s: want 1 Stamp call, got %d", addr, got)
		}
		want, err := storage.IdentityAddress(ch)
		if err != nil {
			t.Fatal(err)
		}
		if got := stamper.idAddrFor(addr); !got.Equal(want) {
			t.Fatalf("replica %s: stamped with id address %s, want %s", addr, got, want)
		}
	}
}

// pushedReplicas reads the pusher feed until want chunks were pushed, checks
// each one already exists locally and returns the dispersed replicas among them.
func pushedReplicas(ctx context.Context, feed <-chan *pusher.Op, cs storage.ChunkStore, want int) (func() (map[string]struct{}, error), <-chan struct{}) {
	var (
		mu     sync.Mutex
		seen   = make(map[string]struct{})
		feedEr error
		done   = make(chan struct{})
	)
	go func() {
		defer close(done)
		count := 0
		for op := range feed {
			has, err := cs.Has(ctx, op.Chunk.Address())
			if err == nil && !has {
				err = fmt.Errorf("pushed chunk %s not found locally", op.Chunk.Address())
			}
			if err != nil {
				mu.Lock()
				feedEr = err
				mu.Unlock()
				return
			}
			if isDispersedReplica(op.Chunk) {
				mu.Lock()
				seen[op.Chunk.Address().String()] = struct{}{}
				mu.Unlock()
			}
			count++
			if count == want {
				return
			}
		}
	}()
	return func() (map[string]struct{}, error) {
		mu.Lock()
		defer mu.Unlock()
		out := make(map[string]struct{}, len(seen))
		for a := range seen {
			out[a] = struct{}{}
		}
		return out, feedEr
	}, done
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
		stamper        = newRecordingStamper()
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
	snapshot, done := pushedReplicas(ctx, store.PusherFeed(), chunkStore, trieChunkCount+replicaCount)

	err = s.Reupload(ctx, addr, stamper, redundancy.PARANOID)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("took too long to finish")
	}

	gotReplicas, err := snapshot()
	if err != nil {
		t.Fatal(err)
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

	// The re-uploaded replicas must be exactly the ones the regular upload path
	// produced: same count and same addresses. Asserting only the count would
	// not catch replicas derived from the wrong root chunk.
	assertSameReplicas(t, uploadReplicas, gotReplicas)
	assertReplicaStamps(t, ctx, chunkStore, stamper, gotReplicas)
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

	// Replicas are not walked by traversal, so the re-upload pushes the trie
	// chunks plus a fresh set of replicas.
	wantPushed := int(inmem.count.Load()) - len(uploadReplicas) + replicaCount
	snapshot, done := pushedReplicas(ctx, store.PusherFeed(), chunkStore, wantPushed)

	err = s.Reupload(ctx, addr, stamper, redundancy.PARANOID)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("took too long to finish")
	}

	gotReplicas, err := snapshot()
	if err != nil {
		t.Fatal(err)
	}

	// The re-uploaded replicas must be exactly the ones the upload path derived
	// from the plain content address. If Reupload had derived them from the
	// 64-byte encrypted reference instead, the addresses would differ and this
	// would fail even though the count still matched.
	assertSameReplicas(t, uploadReplicas, gotReplicas)
	assertReplicaStamps(t, ctx, chunkStore, stamper, gotReplicas)

	// Every replica must wrap the plain 32-byte content address's chunk, and
	// replicas.NewPutter derives replica addresses from that same chunk's
	// address (ch.Address()) - so this also proves replica addresses were
	// derived from contentAddr, not the 64-byte encrypted reference. If the
	// reference had not been trimmed before the fix, this lookup would have
	// failed (get root chunk for dispersed replicas) or wrapped the wrong chunk.
	for addrStr := range gotReplicas {
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

// TestStewardManifestReplicas verifies that Reupload re-creates the dispersed
// replicas of every joiner root under a manifest - the manifest nodes and the
// file it references - not only of the top-level reference.
func TestStewardManifestReplicas(t *testing.T) {
	t.Parallel()

	const rLevel = redundancy.PARANOID
	var (
		ctx        = context.Background()
		inmem      = &counter{ChunkStore: inmemchunkstore.New()}
		chunkStore = inmem
		store      = mockstorer.NewWithChunkStore(chunkStore)
		s          = steward.New(store, &localRetriever{ChunkStore: chunkStore}, inmem)
		stamper    = newRecordingStamper()
		data       = make([]byte, 3*swarm.ChunkSize)
	)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}

	fileRef, err := builder.FeedPipeline(ctx, builder.NewPipelineBuilder(ctx, chunkStore, false, rLevel), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	factory := func() pipeline.Interface { return builder.NewPipelineBuilder(ctx, chunkStore, false, rLevel) }
	m, err := manifest.NewDefaultManifest(loadsave.New(chunkStore, chunkStore, factory, rLevel), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Add(ctx, "file.bin", manifest.NewEntry(fileRef, nil)); err != nil {
		t.Fatal(err)
	}
	manifestRef, err := m.Store(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// each pipeline (the file and every manifest node) disperses its own root
	uploadReplicas := inmem.replicaSet()
	if len(uploadReplicas) <= rLevel.GetReplicaCount() {
		t.Fatalf("expected replicas for more than one root, got %d", len(uploadReplicas))
	}

	wantPushed := int(inmem.count.Load())
	snapshot, done := pushedReplicas(ctx, store.PusherFeed(), chunkStore, wantPushed)

	if err := s.Reupload(ctx, manifestRef, stamper, rLevel); err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("took too long to finish")
	}

	gotReplicas, err := snapshot()
	if err != nil {
		t.Fatal(err)
	}
	assertSameReplicas(t, uploadReplicas, gotReplicas)
	assertReplicaStamps(t, ctx, chunkStore, stamper, gotReplicas)
}

// TestStewardSOCReference verifies that re-uploading a single owner chunk
// reference with redundancy succeeds and creates no dispersed replicas.
func TestStewardSOCReference(t *testing.T) {
	t.Parallel()

	var (
		ctx        = context.Background()
		chunkStore = inmemchunkstore.New()
		store      = mockstorer.NewWithChunkStore(chunkStore)
		s          = steward.New(store, &localRetriever{ChunkStore: chunkStore}, chunkStore)
	)

	sch := testingsoc.GenerateMockSOC(t, []byte("soc data")).Chunk()
	if err := chunkStore.Put(ctx, sch); err != nil {
		t.Fatal(err)
	}

	snapshot, done := pushedReplicas(ctx, store.PusherFeed(), chunkStore, 1)

	if err := s.Reupload(ctx, sch.Address(), postagetesting.NewStamper(), redundancy.DefaultUploadLevel); err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("took too long to finish")
	}

	gotReplicas, err := snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(gotReplicas) != 0 {
		t.Fatalf("expected no dispersed replicas for a SOC reference, got %d", len(gotReplicas))
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
