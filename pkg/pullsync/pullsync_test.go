// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/pullsync"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storage"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	mock "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	results []*storer.BinC
	addrs   []swarm.Address
	chunks  []swarm.Chunk
)

func someChunks(i ...int) (c []swarm.Chunk) {
	for _, v := range i {
		c = append(c, chunks[v])
	}
	return c
}

// nolint:gochecknoinits
func init() {
	n := 5
	chunks = make([]swarm.Chunk, n)
	addrs = make([]swarm.Address, n)
	results = make([]*storer.BinC, n)
	for i := 0; i < n; i++ {
		chunks[i] = testingc.GenerateTestRandomChunk()
		addrs[i] = chunks[i].Address()
		stampHash, _ := chunks[i].Stamp().Hash()
		results[i] = &storer.BinC{
			Address:   addrs[i],
			BatchID:   chunks[i].Stamp().BatchID(),
			BinID:     uint64(i),
			StampHash: stampHash,
		}
	}
}

func TestIncoming_WantNone(t *testing.T) {
	t.Parallel()

	var (
		topMost            = uint64(4)
		ps, _              = newPullSync(t, nil, 5, mock.WithSubscribeResp(results, nil), mock.WithChunks(chunks...))
		recorder           = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, clientDb = newPullSync(t, recorder, 0, mock.WithChunks(chunks...))
	)

	topmost, _, err := psClient.Sync(context.Background(), swarm.ZeroAddress, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	if topmost != topMost {
		t.Fatalf("got offer topmost %d but want %d", topmost, topMost)
	}
	if clientDb.PutCalls() > 0 {
		t.Fatal("too many puts")
	}
}

func TestIncoming_ContextTimeout(t *testing.T) {
	t.Parallel()

	var (
		ps, _       = newPullSync(t, nil, 0, mock.WithSubscribeResp(results, nil), mock.WithChunks(chunks...))
		recorder    = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, _ = newPullSync(t, recorder, 0, mock.WithChunks(chunks...))
	)

	ctx, cancel := context.WithTimeout(context.Background(), 0)
	cancel()
	_, _, err := psClient.Sync(ctx, swarm.ZeroAddress, 0, 0)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wanted error %v, got %v", context.DeadlineExceeded, err)
	}
}

func TestIncoming_WantOne(t *testing.T) {
	t.Parallel()

	var (
		topMost            = uint64(4)
		ps, _              = newPullSync(t, nil, 5, mock.WithSubscribeResp(results, nil), mock.WithChunks(chunks...))
		recorder           = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, clientDb = newPullSync(t, recorder, 0, mock.WithChunks(someChunks(1, 2, 3, 4)...))
	)

	topmost, _, err := psClient.Sync(context.Background(), swarm.ZeroAddress, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	if topmost != topMost {
		t.Fatalf("got offer topmost %d but want %d", topmost, topMost)
	}

	// should have all
	haveChunks(t, clientDb, chunks...)
	if clientDb.PutCalls() != 1 {
		t.Fatalf("want 1 puts but got %d", clientDb.PutCalls())
	}
}

func TestIncoming_WantAll(t *testing.T) {
	t.Parallel()

	var (
		topMost            = uint64(4)
		ps, _              = newPullSync(t, nil, 5, mock.WithSubscribeResp(results, nil), mock.WithChunks(chunks...))
		recorder           = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, clientDb = newPullSync(t, recorder, 0)
	)

	topmost, _, err := psClient.Sync(context.Background(), swarm.ZeroAddress, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	if topmost != topMost {
		t.Fatalf("got offer topmost %d but want %d", topmost, topMost)
	}

	// should have all
	haveChunks(t, clientDb, chunks...)
	if p := clientDb.PutCalls(); p != len(chunks) {
		t.Fatalf("want %d puts but got %d", len(chunks), p)
	}
}

func TestIncoming_WantErrors(t *testing.T) {
	t.Parallel()

	tChunks := testingc.GenerateTestRandomChunks(4)
	// add same chunk with a different batch id
	ch := swarm.NewChunk(tChunks[3].Address(), tChunks[3].Data()).WithStamp(postagetesting.MustNewStamp())
	tChunks = append(tChunks, ch)
	// add invalid chunk
	tChunks = append(tChunks, testingc.GenerateTestRandomInvalidChunk())

	tResults := make([]*storer.BinC, len(tChunks))
	for i, c := range tChunks {
		stampHash, err := c.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		tResults[i] = &storer.BinC{
			Address:   c.Address(),
			BatchID:   c.Stamp().BatchID(),
			BinID:     uint64(i + 5), // start from a higher bin id
			StampHash: stampHash,
		}
	}

	putHook := func(c swarm.Chunk) error {
		if c.Address().Equal(tChunks[1].Address()) {
			return storage.ErrOverwriteNewerChunk
		}
		return nil
	}

	validStampErr := errors.New("valid stamp error")
	validStamp := func(c swarm.Chunk) (swarm.Chunk, error) {
		if c.Address().Equal(tChunks[2].Address()) {
			return nil, validStampErr
		}
		return c, nil
	}

	var (
		topMost            = uint64(10)
		ps, _              = newPullSync(t, nil, 20, mock.WithSubscribeResp(tResults, nil), mock.WithChunks(tChunks...))
		recorder           = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, clientDb = newPullSyncWithStamperValidator(t, recorder, 0, validStamp, mock.WithPutHook(putHook))
	)

	topmost, count, err := psClient.Sync(context.Background(), swarm.ZeroAddress, 0, 0)
	for _, e := range []error{storage.ErrOverwriteNewerChunk, validStampErr, swarm.ErrInvalidChunk} {
		if !errors.Is(err, e) {
			t.Fatalf("expected error %v", err)
		}
	}

	if count != 3 {
		t.Fatalf("got %d chunks but want %d", count, 3)
	}

	if topmost != topMost {
		t.Fatalf("got offer topmost %d but want %d", topmost, topMost)
	}

	haveChunks(t, clientDb, append(tChunks[:1], tChunks[3:5]...)...)
	if p := clientDb.PutCalls(); p != len(chunks)-1 {
		t.Fatalf("want %d puts but got %d", len(chunks), p)
	}
}

func TestIncoming_UnsolicitedChunk(t *testing.T) {
	t.Parallel()

	evilAddr := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000666")
	evilData := []byte{0x66, 0x66, 0x66}
	stamp := postagetesting.MustNewStamp()
	evil := swarm.NewChunk(evilAddr, evilData).WithStamp(stamp)

	var (
		ps, _       = newPullSync(t, nil, 5, mock.WithSubscribeResp(results, nil), mock.WithChunks(chunks...), mock.WithEvilChunk(addrs[4], evil))
		recorder    = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, _ = newPullSync(t, recorder, 0)
	)

	_, _, err := psClient.Sync(context.Background(), swarm.ZeroAddress, 0, 0)
	if !errors.Is(err, pullsync.ErrUnsolicitedChunk) {
		t.Fatalf("expected err %v but got %v", pullsync.ErrUnsolicitedChunk, err)
	}
}

func TestMissingChunk(t *testing.T) {
	t.Parallel()

	var (
		zeroChunk   = swarm.NewChunk(swarm.ZeroAddress, nil)
		topMost     = uint64(4)
		ps, _       = newPullSync(t, nil, 5, mock.WithSubscribeResp(results, nil), mock.WithChunks([]swarm.Chunk{zeroChunk}...))
		recorder    = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, _ = newPullSync(t, recorder, 0)
	)

	topmost, count, err := psClient.Sync(context.Background(), swarm.ZeroAddress, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	if topmost != topMost {
		t.Fatalf("got offer topmost %d but want %d", topmost, topMost)
	}
	if count != 0 {
		t.Fatalf("got count %d but want %d", count, 0)
	}
}

func TestGetCursors(t *testing.T) {
	t.Parallel()

	var (
		epochTs     = uint64(time.Now().Unix())
		mockCursors = []uint64{100, 101, 102, 103}
		ps, _       = newPullSync(t, nil, 0, mock.WithCursors(mockCursors, epochTs))
		recorder    = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, _ = newPullSync(t, recorder, 0)
	)

	curs, epoch, err := psClient.GetCursors(context.Background(), swarm.ZeroAddress)
	if err != nil {
		t.Fatal(err)
	}

	if len(curs) != len(mockCursors) {
		t.Fatalf("length mismatch got %d want %d", len(curs), len(mockCursors))
	}

	if epochTs != epoch {
		t.Fatalf("epochs do not match got %d want %d", epoch, epochTs)
	}

	for i, v := range mockCursors {
		if curs[i] != v {
			t.Errorf("cursor mismatch. index %d want %d got %d", i, v, curs[i])
		}
	}
}

func TestGetCursorsError(t *testing.T) {
	t.Parallel()

	var (
		e           = errors.New("erring")
		ps, _       = newPullSync(t, nil, 0, mock.WithCursorsErr(e))
		recorder    = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, _ = newPullSync(t, recorder, 0)
	)

	_, _, err := psClient.GetCursors(context.Background(), swarm.ZeroAddress)
	if err == nil {
		t.Fatal("expected error but got none")
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expect error '%v' but got '%v'", e, err)
	}
}

func haveChunks(t *testing.T, s *mock.ReserveStore, chunks ...swarm.Chunk) {
	t.Helper()
	for _, c := range chunks {
		stampHash, err := c.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		have, err := s.ReserveHas(c.Address(), c.Stamp().BatchID(), stampHash)
		if err != nil {
			t.Fatal(err)
		}
		if !have {
			t.Errorf("storage does not have chunk %s", c.Address())
		}
	}
}

func newPullSync(
	t *testing.T,
	s p2p.Streamer,
	maxPage uint64,
	o ...mock.Option,
) (*pullsync.Syncer, *mock.ReserveStore) {
	t.Helper()

	validStamp := func(ch swarm.Chunk) (swarm.Chunk, error) {
		return ch, nil
	}

	return newPullSyncWithStamperValidator(t, s, maxPage, validStamp, o...)
}

func newPullSyncWithStamperValidator(
	t *testing.T,
	s p2p.Streamer,
	maxPage uint64,
	validStamp postage.ValidStampFn,
	o ...mock.Option,
) (*pullsync.Syncer, *mock.ReserveStore) {
	t.Helper()

	storage := mock.NewReserve(o...)
	logger := log.Noop
	unwrap := func(swarm.Chunk) {}
	socHandler := func(*soc.SOC) {}
	ps := pullsync.New(
		s,
		storage,
		unwrap,
		socHandler,
		validStamp,
		logger,
		maxPage,
	)

	t.Cleanup(func() {
		err := ps.Close()
		if err != nil {
			t.Errorf("failed closing pullsync: %v", err)
		}
	})
	return ps, storage
}
