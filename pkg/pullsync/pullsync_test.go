// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync_test

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/pullsync"
	"github.com/ethersphere/bee/pkg/pullsync/pullstorage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	addrs = []swarm.Address{
		swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000001"),
		swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000002"),
		swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000003"),
		swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000004"),
		swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000005"),
	}
	chunks []swarm.Chunk
)

func someChunks(i ...int) (c []swarm.Chunk) {
	for _, v := range i {
		c = append(c, chunks[v])
	}
	return c
}

func init() {
	chunks = make([]swarm.Chunk, 5)
	for i := 0; i < 5; i++ {
		data := make([]byte, swarm.ChunkSize)
		_, _ = rand.Read(data)
		chunks[i] = swarm.NewChunk(addrs[i], data)
	}
}

// TestIncoming tests that an incoming request for an interval
// is handled correctly when no chunks are available in the interval.
// This means the interval exists but chunks are not there (GCd).
// Expected behavior is that an offer message with the requested
// To value is returned to the requester, but offer.Hashes is empty.
func TestIncoming_WantEmptyInterval(t *testing.T) {
	defer func(i int) {
		*pullsync.MaxPage = i
	}(*pullsync.MaxPage)
	*pullsync.MaxPage = 5

	var (
		mockTopmost        = uint64(5)
		ps, _              = newPullSync(nil, mock.WithIntervalsResp([]swarm.Address{}, mockTopmost, nil))
		recorder           = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, clientDb = newPullSync(recorder)
	)

	topmost, err := psClient.SyncInterval(context.Background(), swarm.ZeroAddress, 0, 0, 5)
	if err != nil {
		t.Fatal(err)
	}

	if topmost != mockTopmost {
		t.Fatalf("got offer topmost %d but want %d", topmost, mockTopmost)
	}

	if clientDb.PutCalls() > 0 {
		t.Fatal("too many puts")
	}
}
func TestIncoming_WantNone(t *testing.T) {
	defer func(i int) {
		*pullsync.MaxPage = i
	}(*pullsync.MaxPage)
	*pullsync.MaxPage = 5

	var (
		mockTopmost        = uint64(5)
		ps, _              = newPullSync(nil, mock.WithIntervalsResp(addrs, mockTopmost, nil), mock.WithChunks(chunks...))
		recorder           = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, clientDb = newPullSync(recorder, mock.WithChunks(chunks...))
	)

	topmost, err := psClient.SyncInterval(context.Background(), swarm.ZeroAddress, 0, 0, 5)
	if err != nil {
		t.Fatal(err)
	}

	if topmost != mockTopmost {
		t.Fatalf("got offer topmost %d but want %d", topmost, mockTopmost)
	}
	if clientDb.PutCalls() > 0 {
		t.Fatal("too many puts")
	}
}

func TestIncoming_WantOne(t *testing.T) {
	defer func(i int) {
		*pullsync.MaxPage = i
	}(*pullsync.MaxPage)
	*pullsync.MaxPage = 5

	var (
		mockTopmost        = uint64(5)
		ps, _              = newPullSync(nil, mock.WithIntervalsResp(addrs, mockTopmost, nil), mock.WithChunks(chunks...))
		recorder           = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, clientDb = newPullSync(recorder, mock.WithChunks(someChunks(1, 2, 3, 4)...))
	)

	topmost, err := psClient.SyncInterval(context.Background(), swarm.ZeroAddress, 0, 0, 5)
	if err != nil {
		t.Fatal(err)
	}

	if topmost != mockTopmost {
		t.Fatalf("got offer topmost %d but want %d", topmost, mockTopmost)
	}

	// should have all
	haveChunks(t, clientDb, addrs...)
	if clientDb.PutCalls() > 1 {
		t.Fatal("too many puts")
	}
}

func TestIncoming_WantAll(t *testing.T) {
	defer func(i int) {
		*pullsync.MaxPage = i
	}(*pullsync.MaxPage)
	*pullsync.MaxPage = 5

	var (
		mockTopmost        = uint64(5)
		ps, _              = newPullSync(nil, mock.WithIntervalsResp(addrs, mockTopmost, nil), mock.WithChunks(chunks...))
		recorder           = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, clientDb = newPullSync(recorder)
	)

	topmost, err := psClient.SyncInterval(context.Background(), swarm.ZeroAddress, 0, 0, 5)
	if err != nil {
		t.Fatal(err)
	}

	if topmost != mockTopmost {
		t.Fatalf("got offer topmost %d but want %d", topmost, mockTopmost)
	}

	// should have all
	haveChunks(t, clientDb, addrs...)
	if p := clientDb.PutCalls(); p != 5 {
		t.Fatalf("want %d puts but got %d", 5, p)
	}
}

func TestIncoming_UnsolicitedChunk(t *testing.T) {
	defer func(i int) {
		*pullsync.MaxPage = i
	}(*pullsync.MaxPage)
	*pullsync.MaxPage = 5

	evilAddr := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000666")
	evilData := []byte{0x66, 0x66, 0x66}
	evil := swarm.NewChunk(evilAddr, evilData)

	var (
		mockTopmost = uint64(5)
		ps, _       = newPullSync(nil, mock.WithIntervalsResp(addrs, mockTopmost, nil), mock.WithChunks(chunks...), mock.WithEvilChunk(addrs[4], evil))
		recorder    = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
		psClient, _ = newPullSync(recorder)
	)

	_, err := psClient.SyncInterval(context.Background(), swarm.ZeroAddress, 0, 0, 5)
	if !errors.Is(err, pullsync.ErrUnsolicitedChunk) {
		t.Fatalf("expected ErrUnsolicitedChunk but got %v", err)
	}
}

func haveChunks(t *testing.T, s *mock.PullStorage, addrs ...swarm.Address) {
	t.Helper()
	for _, a := range addrs {
		have, err := s.Has(context.Background(), a)
		if err != nil {
			t.Fatal(err)
		}
		if !have {
			t.Errorf("storage does not have chunk %s", a)
		}
	}
}

func newPullSync(s p2p.Streamer, o ...mock.Option) (*pullsync.Syncer, *mock.PullStorage) {
	storage := mock.NewPullStorage(o...)
	return pullsync.New(pullsync.Options{Streamer: s, Storage: storage}), storage
}
