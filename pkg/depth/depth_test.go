// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package depth_test

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/depth"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/transaction/backendmock"
	"github.com/ethersphere/bee/v2/pkg/util/abiutil"
	redistabi "github.com/ethersphere/go-storage-incentives-abi/abi"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

const (
	testBlocksPerRound = 152
	testBlockTime      = time.Hour
)

var errRPC = errors.New("rpc failure")

type fakeChain struct {
	t   *testing.T
	abi abi.ABI

	mu         sync.Mutex
	head       uint64
	headErr    error
	logs       []types.Log
	logsErr    error
	claimRound uint64
	callErr    error
	queries    []ethereum.FilterQuery
}

func newBackend(t *testing.T) *fakeChain {
	t.Helper()

	return &fakeChain{
		t:          t,
		abi:        abiutil.MustParseABI(redistabi.MainnetRedistributionABI),
		head:       10000,
		claimRound: 7,
	}
}

func (f *fakeChain) Backend() transaction.Backend {
	return backendmock.New(
		backendmock.WithBlockNumberFunc(func(context.Context) (uint64, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.head, f.headErr
		}),
		backendmock.WithFilterLogsFunc(func(_ context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.queries = append(f.queries, q)
			if f.logsErr != nil {
				return nil, f.logsErr
			}
			var out []types.Log
			for _, l := range f.logs {
				if l.BlockNumber >= q.FromBlock.Uint64() && l.BlockNumber <= q.ToBlock.Uint64() {
					out = append(out, l)
				}
			}
			return out, nil
		}),
		backendmock.WithCallContractFunc(func(_ context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.callErr != nil {
				return nil, f.callErr
			}
			m := f.abi.Methods["currentClaimRound"]
			if !bytes.HasPrefix(msg.Data, m.ID) {
				return nil, errors.New("unexpected call")
			}
			return m.Outputs.Pack(f.claimRound)
		}),
	)
}

func (f *fakeChain) set(fn func(f *fakeChain)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeChain) addLogs(d uint8, blocks ...uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, b := range blocks {
		f.logs = append(f.logs, truthLog(f.t, f.abi, [32]byte{byte(b)}, d, b))
	}
}

func (f *fakeChain) queriesCopy() []ethereum.FilterQuery {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ethereum.FilterQuery(nil), f.queries...)
}

func (f *fakeChain) resetQueries() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = nil
}

// newProvider builds a provider and stops its worker, so the test drives
// Backfill, Poll and CheckHistory synchronously against a quiet backend. The provider
// is not closed, so Wait behaves as in production.
func newProvider(t *testing.T, f *fakeChain) depth.Provider {
	t.Helper()

	a := f.abi
	p := depth.New(f.Backend(), common.HexToAddress("0x01"), a, testBlockTime, testBlocksPerRound, log.Noop)
	t.Cleanup(func() { _ = p.Close() })
	depth.StopWorker(p)
	f.resetQueries()
	return p
}

// value reads the current value of a gauge or counter. The prometheus testutil package is
// not used because it needs an additional module in go.mod.
func value(t *testing.T, c prometheus.Metric) float64 {
	t.Helper()

	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatal(err)
	}
	if g := m.GetGauge(); g != nil {
		return g.GetValue()
	}
	return m.GetCounter().GetValue()
}

func blocks(from, to, step uint64) []uint64 {
	var out []uint64
	for b := from; b <= to; b += step {
		out = append(out, b)
	}
	return out
}

func assertDepth(t *testing.T, p depth.Provider, wantDepth uint8, wantState depth.State) {
	t.Helper()

	d, s := p.Depth()
	if d != wantDepth || s != wantState {
		t.Fatalf("got (%d, %s), want (%d, %s)", d, s, wantDepth, wantState)
	}
}

// knownAt10 returns a chain with ten logs at blocks 8500..9400, mostly depth 10.
func knownAt10(t *testing.T) *fakeChain {
	t.Helper()

	f := newBackend(t)
	bs := blocks(8500, 9400, 100)
	f.addLogs(9, bs[:2]...)
	f.addLogs(10, bs[2:]...)
	return f
}

func TestBackfillKnown(t *testing.T) {
	t.Parallel()

	f := knownAt10(t)
	p := newProvider(t, f)
	ctx := context.Background()

	if err := depth.Backfill(p, ctx); err != nil {
		t.Fatal(err)
	}
	assertDepth(t, p, 10, depth.StateKnown)

	wctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	d, s, err := p.Wait(wctx)
	if err != nil || d != 10 || s != depth.StateKnown {
		t.Fatalf("wait: got (%d, %s, %v)", d, s, err)
	}

	qs := f.queriesCopy()
	if len(qs) == 0 {
		t.Fatal("no queries")
	}
	if got := qs[0].FromBlock.Uint64(); got != 6960 {
		t.Fatalf("first from block: got %d, want 6960", got)
	}
	if got := qs[len(qs)-1].ToBlock.Uint64(); got != 10000 {
		t.Fatalf("last to block: got %d, want 10000", got)
	}
	for i, q := range qs {
		from, to := q.FromBlock.Uint64(), q.ToBlock.Uint64()
		if to-from+1 > 1000 {
			t.Fatalf("query %d spans %d blocks", i, to-from+1)
		}
		if i > 0 && from != qs[i-1].ToBlock.Uint64()+1 {
			t.Fatalf("query %d not contiguous: from %d after %d", i, from, qs[i-1].ToBlock.Uint64())
		}
	}
}

func TestBackfillClampsAtGenesis(t *testing.T) {
	t.Parallel()

	f := newBackend(t)
	f.set(func(f *fakeChain) { f.head = 500 })
	p := newProvider(t, f)

	if err := depth.Backfill(p, context.Background()); err != nil {
		t.Fatal(err)
	}
	qs := f.queriesCopy()
	if len(qs) == 0 {
		t.Fatal("no queries")
	}
	if got := qs[0].FromBlock.Uint64(); got != 0 {
		t.Fatalf("first from block: got %d, want 0", got)
	}
}

func TestBackfillKeepsNewestTen(t *testing.T) {
	t.Parallel()

	f := newBackend(t)
	bs := blocks(8300, 9400, 100)
	f.addLogs(9, bs[:2]...)
	f.addLogs(10, bs[2:]...)
	p := newProvider(t, f)

	if err := depth.Backfill(p, context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := depth.WindowSamples(p); got != 10 {
		t.Fatalf("window size: got %d, want 10", got)
	}
	if got := depth.OldestSampleBlock(p); got != 8500 {
		t.Fatalf("oldest sample block: got %d, want 8500", got)
	}
	assertDepth(t, p, 10, depth.StateKnown)
}

func TestNoHistory(t *testing.T) {
	t.Parallel()

	f := newBackend(t)
	f.set(func(f *fakeChain) { f.claimRound = 0 })
	p := newProvider(t, f)
	ctx := context.Background()

	if err := depth.Backfill(p, ctx); err != nil {
		t.Fatal(err)
	}
	if err := depth.CheckHistory(p, ctx); err != nil {
		t.Fatal(err)
	}
	assertDepth(t, p, 0, depth.StateNoHistory)

	wctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_, s, err := p.Wait(wctx)
	if err != nil || s != depth.StateNoHistory {
		t.Fatalf("wait: got (%s, %v)", s, err)
	}
	if got := value(t, depth.StateGauge(p, depth.StateNoHistory)); got != 1 {
		t.Fatalf("no_history gauge: got %v, want 1", got)
	}
	if got := value(t, depth.StateGauge(p, depth.StateNoData)); got != 0 {
		t.Fatalf("no_data gauge: got %v, want 0", got)
	}
}

func TestNoDataWithHistory(t *testing.T) {
	t.Parallel()

	f := newBackend(t)
	f.addLogs(10, 9000, 9100, 9200)
	p := newProvider(t, f)
	ctx := context.Background()

	if err := depth.Backfill(p, ctx); err != nil {
		t.Fatal(err)
	}
	if err := depth.CheckHistory(p, ctx); err != nil {
		t.Fatal(err)
	}
	assertDepth(t, p, 0, depth.StateNoData)

	wctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, _, err := p.Wait(wctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait: got %v, want deadline exceeded", err)
	}
}

func TestCheckHistoryErrorKeepsNoData(t *testing.T) {
	t.Parallel()

	f := newBackend(t)
	f.set(func(f *fakeChain) { f.callErr = errRPC })
	p := newProvider(t, f)
	ctx := context.Background()

	if err := depth.Backfill(p, ctx); err != nil {
		t.Fatal(err)
	}
	if err := depth.CheckHistory(p, ctx); !errors.Is(err, errRPC) {
		t.Fatalf("check history: got %v, want rpc error", err)
	}
	assertDepth(t, p, 0, depth.StateNoData)
}

func TestBlockNumberErrorKeepsValue(t *testing.T) {
	t.Parallel()

	f := knownAt10(t)
	p := newProvider(t, f)
	ctx := context.Background()

	if err := depth.Backfill(p, ctx); err != nil {
		t.Fatal(err)
	}
	assertDepth(t, p, 10, depth.StateKnown)

	f.set(func(f *fakeChain) { f.headErr = errRPC })
	if err := depth.Poll(p, ctx); !errors.Is(err, errRPC) {
		t.Fatalf("poll: got %v, want rpc error", err)
	}
	assertDepth(t, p, 10, depth.StateKnown)
	if got := value(t, depth.RPCErrorsCounter(p)); got != 1 {
		t.Fatalf("rpc errors: got %v, want 1", got)
	}
}

func TestFilterLogsErrorKeepsValue(t *testing.T) {
	t.Parallel()

	f := knownAt10(t)
	p := newProvider(t, f)
	ctx := context.Background()

	if err := depth.Backfill(p, ctx); err != nil {
		t.Fatal(err)
	}
	f.set(func(f *fakeChain) {
		f.head = 10050
		f.logsErr = errRPC
	})
	if err := depth.Poll(p, ctx); !errors.Is(err, errRPC) {
		t.Fatalf("poll: got %v, want rpc error", err)
	}
	assertDepth(t, p, 10, depth.StateKnown)
	if got := value(t, depth.RPCErrorsCounter(p)); got != 1 {
		t.Fatalf("rpc errors: got %v, want 1", got)
	}
}

func TestPollStepsOneLevel(t *testing.T) {
	t.Parallel()

	f := newBackend(t)
	f.addLogs(9, blocks(8500, 9400, 100)...)
	p := newProvider(t, f)
	ctx := context.Background()

	if err := depth.Backfill(p, ctx); err != nil {
		t.Fatal(err)
	}
	assertDepth(t, p, 9, depth.StateKnown)

	f.addLogs(11, blocks(10100, 11000, 100)...)
	f.set(func(f *fakeChain) { f.head = 10000 + 3040 })

	if err := depth.Poll(p, ctx); err != nil {
		t.Fatal(err)
	}
	assertDepth(t, p, 10, depth.StateKnown)

	if err := depth.Poll(p, ctx); err != nil {
		t.Fatal(err)
	}
	assertDepth(t, p, 11, depth.StateKnown)
}

func TestPollHeadBehindLastBlock(t *testing.T) {
	t.Parallel()

	f := knownAt10(t)
	p := newProvider(t, f)
	ctx := context.Background()

	if err := depth.Backfill(p, ctx); err != nil {
		t.Fatal(err)
	}
	before := len(f.queriesCopy())

	f.set(func(f *fakeChain) { f.head = 9000 })
	if err := depth.Poll(p, ctx); err != nil {
		t.Fatal(err)
	}
	if got := len(f.queriesCopy()); got != before {
		t.Fatalf("queries: got %d, want %d", got, before)
	}
	assertDepth(t, p, 10, depth.StateKnown)
}

func TestCloseThenWait(t *testing.T) {
	t.Parallel()

	f := newBackend(t)
	p := depth.New(f.Backend(), common.HexToAddress("0x01"), f.abi, testBlockTime, testBlocksPerRound, log.Noop)
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, _, err := p.Wait(ctx); !errors.Is(err, depth.ErrClosed) {
		t.Fatalf("wait: got %v, want ErrClosed", err)
	}
}

func TestDepthConcurrentWithPoll(t *testing.T) {
	t.Parallel()

	f := knownAt10(t)
	p := newProvider(t, f)
	ctx := context.Background()

	if err := depth.Backfill(p, ctx); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 20 {
			if err := depth.Poll(p, ctx); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	for range 100 {
		p.Depth()
	}
	wg.Wait()
}

func TestCeiling(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		commitment, capacity uint64
		want                 uint8
	}{
		{1 << 22, 1 << 22, 0},
		{1<<22 + 1, 1 << 22, 1},
		{1 << 32, 1 << 22, 10},
		{3 << 31, 1 << 22, 11},
		{0, 1 << 22, 0},
	} {
		if got := depth.Ceiling(tc.commitment, tc.capacity); got != tc.want {
			t.Errorf("Ceiling(%d, %d): got %d, want %d", tc.commitment, tc.capacity, got, tc.want)
		}
	}
}

func TestSetComparison(t *testing.T) {
	t.Parallel()

	f := newBackend(t)
	p := newProvider(t, f)

	p.SetComparison("salud", 9)
	if got := value(t, depth.NetworkGauge(p, "salud")); got != 9 {
		t.Fatalf("got %v, want 9", got)
	}
}

func TestCheckHistoryErrorThenNoHistory(t *testing.T) {
	t.Parallel()

	f := newBackend(t)
	f.set(func(f *fakeChain) { f.callErr = errRPC })
	p := newProvider(t, f)
	ctx := context.Background()

	if err := depth.Backfill(p, ctx); err != nil {
		t.Fatal(err)
	}
	if err := depth.CheckHistory(p, ctx); !errors.Is(err, errRPC) {
		t.Fatalf("check history: got %v, want rpc error", err)
	}
	assertDepth(t, p, 0, depth.StateNoData)
	if !depth.NeedsHistoryCheck(p) {
		t.Fatal("a failed check must be retried")
	}

	f.set(func(f *fakeChain) {
		f.callErr = nil
		f.claimRound = 0
	})
	if err := depth.CheckHistory(p, ctx); err != nil {
		t.Fatal(err)
	}
	assertDepth(t, p, 0, depth.StateNoHistory)

	wctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, s, err := p.Wait(wctx); err != nil || s != depth.StateNoHistory {
		t.Fatalf("wait: got (%s, %v)", s, err)
	}
}

func TestNoHistoryRevertsToNoDataAndWaitBlocks(t *testing.T) {
	t.Parallel()

	f := newBackend(t)
	f.set(func(f *fakeChain) { f.claimRound = 0 })
	p := newProvider(t, f)
	ctx := context.Background()

	if err := depth.Backfill(p, ctx); err != nil {
		t.Fatal(err)
	}
	if err := depth.CheckHistory(p, ctx); err != nil {
		t.Fatal(err)
	}
	assertDepth(t, p, 0, depth.StateNoHistory)

	f.set(func(f *fakeChain) { f.claimRound = 1 })
	f.addLogs(10, 10100, 10200)
	f.set(func(f *fakeChain) { f.head = 10300 })
	if err := depth.CheckHistory(p, ctx); err != nil {
		t.Fatal(err)
	}
	if err := depth.Poll(p, ctx); err != nil {
		t.Fatal(err)
	}
	assertDepth(t, p, 0, depth.StateNoData)

	wctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, _, err := p.Wait(wctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait: got %v, want deadline exceeded", err)
	}
	if got := value(t, depth.StateGauge(p, depth.StateNoData)); got != 1 {
		t.Fatalf("no_data gauge: got %v, want 1", got)
	}
	if got := value(t, depth.StateGauge(p, depth.StateNoHistory)); got != 0 {
		t.Fatalf("no_history gauge: got %v, want 0", got)
	}
}

func TestLastUpdateAgeWithoutDepthCalls(t *testing.T) {
	t.Parallel()

	f := knownAt10(t)
	p := newProvider(t, f)

	now := time.Unix(1000, 0)
	depth.SetClock(p, func() time.Time { return now })

	if err := depth.Backfill(p, context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(90 * time.Second)
	if got := value(t, depth.LastUpdateAgeGauge(p)); got != 90 {
		t.Fatalf("age: got %v, want 90", got)
	}
}

func TestNeedsHistoryCheck(t *testing.T) {
	t.Parallel()

	f := newBackend(t)
	f.set(func(f *fakeChain) { f.claimRound = 0 })
	p := newProvider(t, f)
	ctx := context.Background()

	if !depth.NeedsHistoryCheck(p) {
		t.Fatal("unknown: want keep checking")
	}
	if err := depth.CheckHistory(p, ctx); err != nil {
		t.Fatal(err)
	}
	if !depth.NeedsHistoryCheck(p) {
		t.Fatal("zero: want keep checking")
	}
	f.set(func(f *fakeChain) { f.claimRound = 3 })
	if err := depth.CheckHistory(p, ctx); err != nil {
		t.Fatal(err)
	}
	if depth.NeedsHistoryCheck(p) {
		t.Fatal("nonzero: want stop")
	}
}
