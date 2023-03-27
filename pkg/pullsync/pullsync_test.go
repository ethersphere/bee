// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/postage"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/pullsync"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	storer "github.com/ethersphere/bee/pkg/storer"
	mock "github.com/ethersphere/bee/pkg/storer/mock"
	"github.com/ethersphere/bee/pkg/swarm"
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
		results[i] = &storer.BinC{
			Address: addrs[i],
			BatchID: chunks[i].Stamp().BatchID(),
			BinID:   uint64(i),
		}
	}
}

func TestIncoming_WantNone(t *testing.T) {
	t.Parallel()

	var (
		topMost            = uint64(4)
		ps, _              = newPullSync(nil, 5, mock.WithSubscribeResp(results, nil), mock.WithChunks(chunks...))
		recorder           = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, clientDb = newPullSync(recorder, 0, mock.WithChunks(chunks...))
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
		ps, _       = newPullSync(nil, 0, mock.WithSubscribeResp(results, nil), mock.WithChunks(chunks...))
		recorder    = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, _ = newPullSync(recorder, 0, mock.WithChunks(chunks...))
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
		ps, _              = newPullSync(nil, 5, mock.WithSubscribeResp(results, nil), mock.WithChunks(chunks...))
		recorder           = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, clientDb = newPullSync(recorder, 0, mock.WithChunks(someChunks(1, 2, 3, 4)...))
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
	if clientDb.PutCalls() > 1 {
		t.Fatal("too many puts")
	}
}

func TestIncoming_WantAll(t *testing.T) {
	t.Parallel()

	var (
		topMost            = uint64(4)
		ps, _              = newPullSync(nil, 5, mock.WithSubscribeResp(results, nil), mock.WithChunks(chunks...))
		recorder           = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, clientDb = newPullSync(recorder, 0)
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
	if p := clientDb.PutCalls(); p != 1 {
		t.Fatalf("want %d puts but got %d", 1, p)
	}
}

func TestIncoming_UnsolicitedChunk(t *testing.T) {
	t.Parallel()

	evilAddr := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000666")
	evilData := []byte{0x66, 0x66, 0x66}
	stamp := postagetesting.MustNewStamp()
	evil := swarm.NewChunk(evilAddr, evilData).WithStamp(stamp)

	var (
		ps, _       = newPullSync(nil, 5, mock.WithSubscribeResp(results, nil), mock.WithChunks(chunks...), mock.WithEvilChunk(addrs[4], evil))
		recorder    = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, _ = newPullSync(recorder, 0)
	)

	_, _, err := psClient.Sync(context.Background(), swarm.ZeroAddress, 0, 0)
	if !errors.Is(err, pullsync.ErrUnsolicitedChunk) {
		t.Fatalf("expected err %v but got %v", pullsync.ErrUnsolicitedChunk, err)
	}
}

func TestGetCursors(t *testing.T) {
	t.Parallel()

	var (
		mockCursors = []uint64{100, 101, 102, 103}
		ps, _       = newPullSync(nil, 0, mock.WithCursors(mockCursors))
		recorder    = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, _ = newPullSync(recorder, 0)
	)

	curs, err := psClient.GetCursors(context.Background(), swarm.ZeroAddress)
	if err != nil {
		t.Fatal(err)
	}

	if len(curs) != len(mockCursors) {
		t.Fatalf("length mismatch got %d want %d", len(curs), len(mockCursors))
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
		ps, _       = newPullSync(nil, 0, mock.WithCursorsErr(e))
		recorder    = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, _ = newPullSync(recorder, 0)
	)

	_, err := psClient.GetCursors(context.Background(), swarm.ZeroAddress)
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
		have, err := s.ReserveHas(c.Address(), c.Stamp().BatchID())
		if err != nil {
			t.Fatal(err)
		}
		if !have {
			t.Errorf("storage does not have chunk %s", c.Address())
		}
	}
}

func newPullSync(s p2p.Streamer, maxPage uint64, o ...mock.Option) (*pullsync.Syncer, *mock.ReserveStore) {
	storage := mock.NewReserve(o...)
	logger := log.Noop
	unwrap := func(swarm.Chunk) {}
	validStamp := func(ch swarm.Chunk, stampBytes []byte) (swarm.Chunk, error) {
		stamp := new(postage.Stamp)
		err := stamp.UnmarshalBinary(stampBytes)
		if err != nil {
			return nil, err
		}
		return ch.WithStamp(stamp), nil
	}
	return pullsync.New(
		s,
		storage,
		unwrap,
		validStamp,
		logger,
		maxPage,
	), storage
}
