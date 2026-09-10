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

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/manifest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/mock"
	"github.com/ethersphere/bee/v2/pkg/soc"
	soctesting "github.com/ethersphere/bee/v2/pkg/soc/testing"
	"github.com/ethersphere/bee/v2/pkg/steward"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type counter struct {
	storage.ChunkStore
	count atomic.Int32
}

func (c *counter) Put(ctx context.Context, ch swarm.Chunk) (err error) {
	c.count.Add(1)
	return c.ChunkStore.Put(ctx, ch)
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

	pipe := builder.NewPipelineBuilder(ctx, chunkStore, false, redundancy.NONE)
	addr, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	chunkCount := int(inmem.count.Load())
	replicaCount := redundancy.PARANOID.GetReplicaCount()
	wantPushed := chunkCount + replicaCount
	done := make(chan struct{})
	errc := make(chan error, 1)
	replicaAddrs := make(map[string]struct{})
	var replicaMu sync.Mutex
	go func() {
		defer close(done)
		count := 0
		for op := range store.PusherFeed() {
			// Dispersed replicas are newly minted single owner chunks, so unlike
			// the trie chunks they are not expected in the local store. Record
			// them and move on without persisting: putting them into the store
			// the retriever reads from would let IsRetrievable's speculative
			// replica fetches succeed and blur the retrieved-chunk count below.
			if sch, err := soc.FromChunk(op.Chunk); err == nil && bytes.Equal(sch.OwnerAddress(), swarm.ReplicasOwner) {
				replicaMu.Lock()
				replicaAddrs[op.Chunk.Address().String()] = struct{}{}
				replicaMu.Unlock()

				count++
				if count == wantPushed {
					return
				}
				continue
			}

			// Every other pushed chunk must be one the steward actually holds.
			has, err := chunkStore.Has(ctx, op.Chunk.Address())
			if err != nil || !has {
				if !has {
					err = fmt.Errorf("chunk %s not found", op.Chunk.Address())
				}
				select {
				case errc <- err:
				default:
				}
				return
			}

			count++
			if count == wantPushed {
				return
			}
		}
	}()

	// Reupload is run in its own goroutine and guarded by the same deadline: the
	// feed goroutine stops consuming once it has seen wantPushed chunks, so any
	// extra push would block Reupload forever on the unbuffered pusher channel.
	// Guarding only the wait below would let that hang until the go test timeout
	// kills the whole package instead of reporting a clean failure here.
	reuploadErr := make(chan error, 1)
	go func() { reuploadErr <- s.Reupload(ctx, addr, stamper, redundancy.PARANOID) }()

	select {
	case err = <-reuploadErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reupload took too long to finish, it is likely blocked pushing more chunks than expected")
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

	// IsRetrievable's root fetch goes through joiner -> replicas.NewGetter, which
	// races the root address against speculative replica fetches and abandons the
	// losers without waiting for them (see replicas/getter.go). Those goroutines
	// keep writing to retrievedChunks after IsRetrievable has returned, so the
	// read has to take the same lock they do.
	count := localRetrieval.retrievedCount()
	if count != chunkCount {
		t.Fatalf("unexpected no of unique chunks retrieved: want %d have %d", chunkCount, count)
	}

	replicaMu.Lock()
	gotReplicas := len(replicaAddrs)
	replicaMu.Unlock()
	if gotReplicas != replicaCount {
		t.Fatalf("unexpected no of dispersed replicas re-uploaded: want %d have %d", replicaCount, gotReplicas)
	}
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

// TestStewardEncryptedReference verifies that Reupload handles an encrypted
// reference (address + decryption key). The reference has to be trimmed to its
// 32-byte content address before the root chunk is looked up, because that is
// what the chunk store is keyed on. Note the replica addresses themselves would
// be the same either way: replicator.replicate copies the reference into a fixed
// 32-byte id, so the trailing key bytes never reach the derivation.
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

	pipe := builder.NewPipelineBuilder(ctx, chunkStore, true, redundancy.NONE)
	addr, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(addr.Bytes()) != swarm.HashSize+32 {
		t.Fatalf("expected an encrypted reference of length %d, got %d", swarm.HashSize+32, len(addr.Bytes()))
	}

	replicaCount := redundancy.PARANOID.GetReplicaCount()
	contentAddr := swarm.NewAddress(addr.Bytes()[:swarm.HashSize])

	replicaAddrs := make(map[string]struct{})
	var replicaMu sync.Mutex
	done := make(chan struct{})
	errc := make(chan error, 1)
	wantPushed := int(inmem.count.Load()) + replicaCount
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
			if sch, err := soc.FromChunk(op.Chunk); err == nil && bytes.Equal(sch.OwnerAddress(), swarm.ReplicasOwner) {
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
	gotReplicas := len(replicaAddrs)
	replicaMu.Unlock()
	if gotReplicas != replicaCount {
		t.Fatalf("unexpected no of dispersed replicas re-uploaded: want %d have %d", replicaCount, gotReplicas)
	}

	// Every replica must wrap the chunk at the plain 32-byte content address.
	// Without the trim the root chunk lookup itself fails, which is what this
	// guards - strictAddressChunkStore above rejects a non-32-byte Get so the
	// failure surfaces here rather than being masked by inmemchunkstore's
	// silent truncation.
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

func (lr *localRetriever) retrievedCount() int {
	lr.mu.Lock()
	defer lr.mu.Unlock()
	return len(lr.retrievedChunks)
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

// TestStewardSOCRoot covers a single owner chunk reference - a feed update or a
// GSOC payload. traversal.Traverse supports a SOC root and the API documents the
// stewardship reference as being of any type, so re-uploading one must succeed.
// It is checked at redundancy.DefaultUploadLevel because that is what the API
// falls back to when a client sends no Swarm-Redundancy-Level header, i.e. the
// default path rather than an exotic one. A SOC carries no dispersed replicas,
// so there is simply nothing for the replica step to restore.
func TestStewardSOCRoot(t *testing.T) {
	t.Parallel()

	var (
		ctx        = context.Background()
		chunkStore = inmemchunkstore.New()
		store      = mockstorer.NewWithChunkStore(chunkStore)
		s          = steward.New(store, &localRetriever{ChunkStore: chunkStore}, chunkStore)
		stamper    = postagetesting.NewStamper()
	)

	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 64)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	socChunk := soctesting.GenerateMockSocWithSigner(t, data, crypto.NewDefaultSigner(privKey)).Chunk()
	if err := chunkStore.Put(ctx, socChunk); err != nil {
		t.Fatal(err)
	}

	pushed := make(chan swarm.Chunk, 8)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case op := <-store.PusherFeed():
				select {
				case pushed <- op.Chunk:
				default:
				}
			case <-stop:
				return
			}
		}
	}()

	if err := s.Reupload(ctx, socChunk.Address(), stamper, redundancy.DefaultUploadLevel); err != nil {
		t.Fatalf("re-uploading a single owner chunk reference: %v", err)
	}

	select {
	case ch := <-pushed:
		if !ch.Address().Equal(socChunk.Address()) {
			t.Fatalf("pushed chunk %s, want the soc %s", ch.Address(), socChunk.Address())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("soc chunk was not re-uploaded")
	}
}

// TestStewardManifestPerFileReplicas covers a bzz-shaped upload, where the file
// and the manifest wrapping it go through separate pipelines and each ends up
// with dispersed replicas of its own root chunk. Re-uploading the manifest
// reference has to restore both sets: a GET /bzz/{ref}/{path} joins the *file*
// reference through replicas.NewGetter, so restoring only the top level
// reference leaves the download with no replica fallback.
func TestStewardManifestPerFileReplicas(t *testing.T) {
	t.Parallel()

	var (
		ctx        = context.Background()
		chunkStore = inmemchunkstore.New()
		store      = mockstorer.NewWithChunkStore(chunkStore)
		s          = steward.New(store, &localRetriever{ChunkStore: chunkStore}, chunkStore)
		stamper    = postagetesting.NewStamper()
		rLevel     = redundancy.MEDIUM
	)

	data := make([]byte, 3*4096)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}

	// Upload the way pkg/api/bzz.go does: the file first, then a manifest
	// pointing at it, each through its own pipeline.
	filePipe := builder.NewPipelineBuilder(ctx, chunkStore, false, redundancy.NONE)
	fileRoot, err := builder.FeedPipeline(ctx, filePipe, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	factory := func() pipeline.Interface {
		return builder.NewPipelineBuilder(ctx, chunkStore, false, redundancy.NONE)
	}
	ls := loadsave.New(chunkStore, chunkStore, factory, redundancy.NONE)
	mf, err := manifest.NewDefaultManifest(ls, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := mf.Add(ctx, "data.bin", manifest.NewEntry(fileRoot, nil)); err != nil {
		t.Fatal(err)
	}
	manifestRoot, err := mf.Store(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var (
		mu      sync.Mutex
		wrapped = make(map[string]int)
	)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case op := <-store.PusherFeed():
				if sch, err := soc.FromChunk(op.Chunk); err == nil && bytes.Equal(sch.OwnerAddress(), swarm.ReplicasOwner) {
					mu.Lock()
					wrapped[sch.WrappedChunk().Address().String()]++
					mu.Unlock()
				}
			case <-stop:
				return
			}
		}
	}()

	if err := s.Reupload(ctx, manifestRoot, stamper, rLevel); err != nil {
		t.Fatal(err)
	}

	want := rLevel.GetReplicaCount()
	deadline := time.After(3 * time.Second)
	for {
		mu.Lock()
		gotFile, gotManifest := wrapped[fileRoot.String()], wrapped[manifestRoot.String()]
		mu.Unlock()
		if gotFile >= want && gotManifest >= want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("dispersed replicas re-uploaded: file root %s got %d want %d, manifest root %s got %d want %d",
				fileRoot, gotFile, want, manifestRoot, gotManifest, want)
		case <-time.After(20 * time.Millisecond):
		}
	}
}
