// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"testing/synctest"
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
	for i := range n {
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
	synctest.Test(t, func(t *testing.T) {
		var (
			topMost            = uint64(4)
			ps, _              = newPullSync(t, nil, 5, mock.WithSubscribeResp(results, nil), mock.WithChunks(chunks...), mock.WithCursors([]uint64{topMost}, 0))
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
	})
}

func TestIncoming_ContextTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var (
			ps, _       = newPullSync(t, nil, 0, mock.WithSubscribeResp(results, nil), mock.WithChunks(chunks...), mock.WithCursors([]uint64{4}, 0))
			recorder    = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
			psClient, _ = newPullSync(t, recorder, 0, mock.WithChunks(chunks...))
		)

		ctx, cancel := context.WithTimeout(context.Background(), 0)
		cancel()
		_, _, err := psClient.Sync(ctx, swarm.ZeroAddress, 0, 0)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("wanted error %v, got %v", context.DeadlineExceeded, err)
		}
	})
}

func TestIncoming_WantOne(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var (
			topMost            = uint64(4)
			ps, _              = newPullSync(t, nil, 5, mock.WithSubscribeResp(results, nil), mock.WithChunks(chunks...), mock.WithCursors([]uint64{topMost}, 0))
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
	})
}

func TestIncoming_WantAll(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var (
			topMost            = uint64(4)
			ps, _              = newPullSync(t, nil, 5, mock.WithSubscribeResp(results, nil), mock.WithChunks(chunks...), mock.WithCursors([]uint64{topMost}, 0))
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
	})
}

func TestIncoming_WantErrors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
			ps, _              = newPullSync(t, nil, 20, mock.WithSubscribeResp(tResults, nil), mock.WithChunks(tChunks...), mock.WithCursors([]uint64{topMost}, 0))
			recorder           = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
			psClient, clientDb = newPullSyncWithStamperValidator(t, recorder, 0, validStamp, mock.WithPutHook(putHook))
		)

		topmost, count, err := psClient.Sync(context.Background(), swarm.ZeroAddress, 0, 0)
		if err != nil {
			t.Fatal(err)
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
	})
}

func TestIncoming_UnsolicitedChunk(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		evilAddr := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000666")
		evilData := []byte{0x66, 0x66, 0x66}
		stamp := postagetesting.MustNewStamp()
		evil := swarm.NewChunk(evilAddr, evilData).WithStamp(stamp)

		var (
			ps, _       = newPullSync(t, nil, 5, mock.WithSubscribeResp(results, nil), mock.WithChunks(chunks...), mock.WithEvilChunk(addrs[4], evil), mock.WithCursors([]uint64{4}, 0))
			recorder    = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
			psClient, _ = newPullSync(t, recorder, 0)
		)

		_, _, err := psClient.Sync(context.Background(), swarm.ZeroAddress, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestMissingChunk(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var (
			zeroChunk   = swarm.NewChunk(swarm.ZeroAddress, nil)
			topMost     = uint64(4)
			ps, _       = newPullSync(t, nil, 5, mock.WithSubscribeResp(results, nil), mock.WithChunks([]swarm.Chunk{zeroChunk}...), mock.WithCursors([]uint64{topMost}, 0))
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
	})
}

func TestGetCursors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
	})
}

func TestGetCursorsError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
	})
}

// TestSync_LiveChunkTopCappedAtCursor verifies that a live chunk with a BinID
// far beyond the downstream peer's historical cursor does not inflate offer.Topmost.
// Without the cap, the upstream peer would advance its interval to the live
// chunk's BinID, permanently skipping the historical range in between.
func TestSync_LiveChunkTopCappedAtCursor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		liveChunk := testingc.GenerateTestRandomChunk()
		stampHash, err := liveChunk.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}

		// The subscribe response contains only the live chunk at a high BinID.
		// The downstream peer's historical cursor is set much lower.
		const liveBinID = uint64(100)
		const historicalCursor = uint64(10)
		liveResult := []*storer.BinC{{
			Address:   liveChunk.Address(),
			BatchID:   liveChunk.Stamp().BatchID(),
			BinID:     liveBinID,
			StampHash: stampHash,
		}}

		var (
			ps, _       = newPullSync(t, nil, 10, mock.WithSubscribeResp(liveResult, nil), mock.WithChunks(liveChunk), mock.WithCursors([]uint64{historicalCursor}, 0))
			recorder    = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
			psClient, _ = newPullSync(t, recorder, 0)
		)

		topmost, _, err := psClient.Sync(context.Background(), swarm.ZeroAddress, 0, 1)
		if err != nil {
			t.Fatal(err)
		}
		if topmost != historicalCursor {
			t.Fatalf("topmost: got %d, want %d (live BinID %d must not inflate topmost)", topmost, historicalCursor, liveBinID)
		}
	})
}

// TestSync_HistoricalGapReturnsEmptyOfferAtBoundary verifies that when the
// server's first available chunk has a BinID beyond the requested start, the
// server returns an empty offer with Topmost set to firstBinID-1. This lets
// the client advance its interval to the gap boundary without silently marking
// BinIDs that may exist on other peers as synced.
func TestSync_HistoricalGapReturnsEmptyOfferAtBoundary(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		chunk := testingc.GenerateTestRandomChunk()
		stampHash, err := chunk.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}

		// Server holds one chunk at BinID 5; start=2 creates a gap at [2,4].
		const firstBinID = uint64(5)
		const cursor = uint64(5)
		result := []*storer.BinC{{
			Address:   chunk.Address(),
			BatchID:   chunk.Stamp().BatchID(),
			BinID:     firstBinID,
			StampHash: stampHash,
		}}

		var (
			ps, _       = newPullSync(t, nil, 10, mock.WithSubscribeResp(result, nil), mock.WithChunks(chunk), mock.WithCursors([]uint64{cursor}, 0))
			recorder    = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
			psClient, _ = newPullSync(t, recorder, 0)
		)

		topmost, count, err := psClient.Sync(context.Background(), swarm.ZeroAddress, 0, 2)
		if err != nil {
			t.Fatal(err)
		}
		// Empty offer: gap boundary is firstBinID-1.
		if topmost != firstBinID-1 {
			t.Fatalf("topmost: got %d, want %d (gap boundary)", topmost, firstBinID-1)
		}
		if count != 0 {
			t.Fatalf("count: got %d, want 0 (no chunks in gap)", count)
		}
	})
}

// TestSync_MidOfferGapCapsAtContiguousTopmost verifies that when the
// server's offer contains an internal gap (chunks at BinIDs {3,7,11} with
// start=3), Topmost is capped at the contiguous Topmost (3) so the client's
// interval does not advance past the gap. All chunks are still included in
// the offer for eager storage — no chunk data is retransmitted on later rounds.
func TestSync_MidOfferGapCapsAtContiguousTopmost(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ch1 := testingc.GenerateTestRandomChunk()
		ch2 := testingc.GenerateTestRandomChunk()
		ch3 := testingc.GenerateTestRandomChunk()

		makeResult := func(c swarm.Chunk, binID uint64) *storer.BinC {
			h, err := c.Stamp().Hash()
			if err != nil {
				t.Fatal(err)
			}
			return &storer.BinC{Address: c.Address(), BatchID: c.Stamp().BatchID(), BinID: binID, StampHash: h}
		}

		// BinIDs: 3, 7, 11 — gaps at [4,6] and [8,10].
		results := []*storer.BinC{makeResult(ch1, 3), makeResult(ch2, 7), makeResult(ch3, 11)}
		const cursor = uint64(11)

		var (
			ps, _        = newPullSync(t, nil, 10, mock.WithSubscribeResp(results, nil), mock.WithChunks(ch1, ch2, ch3), mock.WithCursors([]uint64{cursor}, 0))
			recorder     = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
			psClient, db = newPullSync(t, recorder, 0)
		)

		topmost, count, err := psClient.Sync(context.Background(), swarm.ZeroAddress, 0, 3)
		if err != nil {
			t.Fatal(err)
		}
		// Topmost must be the contiguous Topmost (3), not the max BinID (11).
		if topmost != 3 {
			t.Fatalf("topmost: got %d, want 3 (contiguous Topmost)", topmost)
		}
		// All three chunks must be delivered and stored in this single round trip.
		if count != 3 {
			t.Fatalf("count: got %d, want 3 (all chunks delivered eagerly)", count)
		}
		haveChunks(t, db, ch1, ch2, ch3)
	})
}

// TestSync_Start0_SkipsGapDetection confirms that when start=0 the leading-gap
// check and the contiguous-Topmost cap are both skipped (both are guarded by
// start > 0). Even though the server's first chunk is at BinID=5, the offer is
// non-empty and topmost equals the historical cursor.
func TestSync_Start0_SkipsGapDetection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ch := testingc.GenerateTestRandomChunk()
		stampHash, err := ch.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		const binID = uint64(5)
		result := []*storer.BinC{{
			Address:   ch.Address(),
			BatchID:   ch.Stamp().BatchID(),
			BinID:     binID,
			StampHash: stampHash,
		}}

		var (
			ps, _       = newPullSync(t, nil, 10, mock.WithSubscribeResp(result, nil), mock.WithChunks(ch), mock.WithCursors([]uint64{binID}, 0))
			recorder    = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
			psClient, _ = newPullSync(t, recorder, 0)
		)

		topmost, count, err := psClient.Sync(context.Background(), swarm.ZeroAddress, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		// Neither gap check fires for start=0; topmost must equal the cursor.
		if topmost != binID {
			t.Fatalf("topmost: got %d, want %d (gap detection must be skipped for start=0)", topmost, binID)
		}
		if count != 1 {
			t.Fatalf("count: got %d, want 1", count)
		}
	})
}

// TestSync_BinBeyondCursors_StallsInterval verifies the safe-stall behaviour
// when bin is beyond the cursors slice returned by ReserveLastBinIDs.
// historicalCursor defaults to 0, so the cursor cap sets topmost=0 for any
// non-empty offer. The client stores the chunk but does not advance the
// interval (top=0 < start=1 in the puller guard).
func TestSync_BinBeyondCursors_StallsInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ch := testingc.GenerateTestRandomChunk()
		stampHash, err := ch.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		result := []*storer.BinC{{
			Address:   ch.Address(),
			BatchID:   ch.Stamp().BatchID(),
			BinID:     1,
			StampHash: stampHash,
		}}

		// Cursors slice has only one entry (bin=0). Syncing bin=1 leaves
		// historicalCursor=0, triggering the safe-stall path.
		var (
			ps, _       = newPullSync(t, nil, 10, mock.WithSubscribeResp(result, nil), mock.WithChunks(ch), mock.WithCursors([]uint64{10}, 0))
			recorder    = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
			psClient, _ = newPullSync(t, recorder, 0)
		)

		topmost, count, err := psClient.Sync(context.Background(), swarm.ZeroAddress, 1, 1)
		if err != nil {
			t.Fatal(err)
		}
		// historicalCursor=0 for bin=1 → cursor cap fires → topmost=0.
		if topmost != 0 {
			t.Fatalf("topmost: got %d, want 0 (bin beyond cursors must stall interval)", topmost)
		}
		// The chunk is still stored; only interval advancement is suppressed.
		if count != 1 {
			t.Fatalf("count: got %d, want 1 (chunk must be stored despite topmost=0)", count)
		}
	})
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
