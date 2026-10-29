// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package depth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"math/bits"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/log"
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/prometheus/client_golang/prometheus"
)

// pageSize is the maximum number of blocks in one FilterLogs query.
const pageSize = 1000

// ErrClosed is returned by Wait when the provider is closed before a value is available.
var ErrClosed = errors.New("depth: provider closed")

// State is the availability of the depth value.
type State uint8

const (
	StateNoData State = iota + 1
	StateNoHistory
	StateKnown
)

func (s State) String() string {
	switch s {
	case StateNoData:
		return "no_data"
	case StateNoHistory:
		return "no_history"
	case StateKnown:
		return "known"
	}
	return "unknown"
}

var allStates = []State{StateNoData, StateNoHistory, StateKnown}

// ChainBackend is the subset of transaction.Backend the provider needs.
type ChainBackend interface {
	BlockNumber(context.Context) (uint64, error)
	FilterLogs(context.Context, ethereum.FilterQuery) ([]types.Log, error)
	CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error)
}

// Provider exposes the network depth estimated from the redistribution contract.
type Provider interface {
	Depth() (depth uint8, state State)
	Wait(ctx context.Context) (depth uint8, state State, err error)
	// SetComparison sets bee_depth_network{source} for a source the provider does not compute
	// itself ("salud", "batchstore_ceiling").
	SetComparison(source string, depth uint8)
	Metrics() []prometheus.Collector
	io.Closer
}

type provider struct {
	backend        ChainBackend
	address        common.Address
	abi            abi.ABI
	topic          common.Hash
	blockTime      time.Duration
	blocksPerRound uint64
	logger         log.Logger
	metrics        metrics

	mu             sync.Mutex
	est            *estimator
	state          State
	lastUpdate     time.Time
	lastBlock      uint64
	backfilled     bool
	claimRoundZero bool
	ready          chan struct{}
	readyClosed    bool

	cancel    context.CancelFunc
	closing   chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

// New builds the provider and starts its worker. address is the resolved redistribution
// contract address. blockTime and blocksPerRound size every interval.
func New(backend ChainBackend, address common.Address, contractABI abi.ABI, blockTime time.Duration, blocksPerRound uint64, logger log.Logger) Provider {
	ctx, cancel := context.WithCancel(context.Background())
	p := &provider{
		backend:        backend,
		address:        address,
		abi:            contractABI,
		topic:          contractABI.Events["TruthSelected"].ID,
		blockTime:      blockTime,
		blocksPerRound: blocksPerRound,
		logger:         logger.WithName("depth").Register(),
		metrics:        newMetrics(),
		est:            newEstimator(),
		state:          StateNoData,
		ready:          make(chan struct{}),
		cancel:         cancel,
		closing:        make(chan struct{}),
		done:           make(chan struct{}),
	}
	p.setStateGaugesLocked()

	go p.worker(ctx)

	return p
}

// Ceiling returns ceil(log2(commitment / capacity)), or 0 when commitment <= capacity.
func Ceiling(commitment, capacity uint64) uint8 {
	if capacity == 0 || commitment <= capacity {
		return 0
	}
	// ceil(log2(ceil(commitment / capacity))) equals ceil(log2(commitment / capacity)).
	ratio := (commitment-1)/capacity + 1
	return uint8(bits.Len64(ratio - 1))
}

func (p *provider) Depth() (uint8, State) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.lastUpdate.IsZero() {
		p.metrics.LastUpdateAge.Set(time.Since(p.lastUpdate).Seconds())
	}
	d, _ := p.est.depth()
	return d, p.state
}

func (p *provider) Wait(ctx context.Context) (uint8, State, error) {
	select {
	case <-p.ready:
		d, s := p.Depth()
		return d, s, nil
	default:
	}

	select {
	case <-p.ready:
		d, s := p.Depth()
		return d, s, nil
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	case <-p.closing:
		return 0, 0, ErrClosed
	}
}

func (p *provider) SetComparison(source string, depth uint8) {
	p.metrics.Network.WithLabelValues(source).Set(float64(depth))
}

func (p *provider) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(p.metrics)
}

// Close stops the worker and waits for it to exit. It is idempotent.
func (p *provider) Close() error {
	p.closeOnce.Do(func() {
		close(p.closing)
		p.cancel()
	})
	<-p.done
	return nil
}

func (p *provider) worker(ctx context.Context) {
	defer close(p.done)

	if !p.backfillWithRetry(ctx) {
		return
	}
	if p.needsHistoryCheck(true) {
		if err := p.checkHistory(ctx); err != nil && ctx.Err() == nil {
			p.logger.Warning("depth: check history failed", "error", err)
		}
	}

	ticker := time.NewTicker(p.blockTime * time.Duration(p.blocksPerRound))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		if p.needsHistoryCheck(false) {
			if err := p.checkHistory(ctx); err != nil && ctx.Err() == nil {
				p.logger.Warning("depth: check history failed", "error", err)
			}
		}
		if err := p.poll(ctx); err != nil && ctx.Err() == nil {
			p.logger.Warning("depth: poll failed", "error", err)
		}
	}
}

// needsHistoryCheck reports whether checkHistory should run: only while the value is not
// known, and, after the first check, only while the last result was a zero claim round.
func (p *provider) needsHistoryCheck(first bool) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == StateKnown {
		return false
	}
	return first || p.claimRoundZero
}

// backfillWithRetry retries backfill with a doubling backoff until it succeeds. It returns
// false when ctx is done first.
func (p *provider) backfillWithRetry(ctx context.Context) bool {
	backoff := p.blockTime
	limit := p.blockTime * time.Duration(p.blocksPerRound)
	for {
		err := p.backfill(ctx)
		if err == nil {
			return true
		}
		if ctx.Err() != nil {
			return false
		}
		p.logger.Warning("depth: backfill failed", "error", err, "retry_in", backoff)

		t := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			t.Stop()
			return false
		case <-t.C:
		}
		backoff = min(backoff*2, limit)
	}
}

// backfill loads the logs of the last windowRounds rounds.
func (p *provider) backfill(ctx context.Context) error {
	head, err := p.blockNumber(ctx)
	if err != nil {
		return err
	}

	var from uint64
	if span := windowRounds * p.blocksPerRound; head > span {
		from = head - span
	}
	if err := p.fetch(ctx, from, head); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.lastBlock = head
	p.backfilled = true
	p.applyLocked(head)

	return nil
}

// poll loads the logs since the last processed block.
func (p *provider) poll(ctx context.Context) error {
	p.mu.Lock()
	backfilled, last := p.backfilled, p.lastBlock
	p.mu.Unlock()

	if !backfilled {
		return p.backfill(ctx)
	}

	head, err := p.blockNumber(ctx)
	if err != nil {
		return err
	}
	if head < last {
		// the node behind us answered with an older head; keep what we have
		p.logger.Debug("head behind last processed block", "head", head, "last_block", last)
		return nil
	}
	if head > last {
		if err := p.fetch(ctx, last+1, head); err != nil {
			return err
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.lastBlock = head
	p.applyLocked(head)

	return nil
}

// checkHistory asks the contract for the current claim round. Zero means the network has
// no redistribution history.
func (p *provider) checkHistory(ctx context.Context) error {
	data, err := p.abi.Pack("currentClaimRound")
	if err != nil {
		return fmt.Errorf("depth: pack currentClaimRound: %w", err)
	}
	result, err := p.backend.CallContract(ctx, ethereum.CallMsg{To: &p.address, Data: data}, nil)
	if err != nil {
		p.rpcError(ctx)
		return fmt.Errorf("depth: call currentClaimRound: %w", err)
	}
	results, err := p.abi.Unpack("currentClaimRound", result)
	if err != nil {
		return fmt.Errorf("depth: unpack currentClaimRound: %w", err)
	}
	if len(results) == 0 {
		return errors.New("depth: unpack currentClaimRound: no results")
	}
	round, ok := results[0].(uint64)
	if !ok {
		return fmt.Errorf("depth: unexpected currentClaimRound type %T", results[0])
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.claimRoundZero = round == 0
	p.evaluateTooFewLocked()

	return nil
}

func (p *provider) blockNumber(ctx context.Context) (uint64, error) {
	head, err := p.backend.BlockNumber(ctx)
	if err != nil {
		p.rpcError(ctx)
		return 0, fmt.Errorf("depth: block number: %w", err)
	}
	return head, nil
}

func (p *provider) rpcError(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	p.metrics.RPCErrors.Inc()
}

// fetch loads the logs of [from, to] in pages and feeds them to the estimator.
func (p *provider) fetch(ctx context.Context, from, to uint64) error {
	for start := from; start <= to; {
		end := min(start+pageSize-1, to)

		logs, err := p.backend.FilterLogs(ctx, ethereum.FilterQuery{
			FromBlock: new(big.Int).SetUint64(start),
			ToBlock:   new(big.Int).SetUint64(end),
			Addresses: []common.Address{p.address},
			Topics:    [][]common.Hash{{p.topic}},
		})
		if err != nil {
			p.rpcError(ctx)
			return fmt.Errorf("depth: filter logs [%d, %d]: %w", start, end, err)
		}
		p.addLogs(logs)

		if end == to {
			break
		}
		start = end + 1
	}
	return nil
}

func (p *provider) addLogs(logs []types.Log) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, l := range logs {
		s, err := decodeTruthSelected(&p.abi, p.topic, l)
		if err != nil {
			p.logger.Debug("skip undecodable log", "error", err, "block", l.BlockNumber)
			continue
		}
		if s.depth > swarm.MaxPO {
			p.metrics.CandidateRejected.WithLabelValues("above_max_po").Inc()
			continue
		}
		p.est.add(s)
	}
}

// applyLocked prunes the window, updates the estimate and the state. p.mu must be held.
func (p *provider) applyLocked(head uint64) {
	p.est.prune(head, p.blocksPerRound)

	switch p.est.update() {
	case updateFirst:
		p.setStateLocked(StateKnown)
	case updateNoMajority:
		p.metrics.CandidateRejected.WithLabelValues("no_majority").Inc()
	case updateTooFew:
		p.evaluateTooFewLocked()
	}

	p.lastUpdate = time.Now()
	p.metrics.LastUpdateAge.Set(0)
	p.metrics.WindowSamples.Set(float64(p.est.size()))
	if d, ok := p.est.depth(); ok {
		p.metrics.Network.WithLabelValues("truth_mode").Set(float64(d))
	}
	if n := len(p.est.samples); n > 0 {
		p.metrics.Network.WithLabelValues("latest_truth").Set(float64(p.est.samples[n-1].depth))
	}
}

// evaluateTooFewLocked sets no_history or no_data while the value is not known.
func (p *provider) evaluateTooFewLocked() {
	if p.state == StateKnown {
		return
	}
	if p.claimRoundZero && p.est.size() < minSamples {
		p.setStateLocked(StateNoHistory)
		return
	}
	p.setStateLocked(StateNoData)
}

func (p *provider) setStateLocked(s State) {
	if p.state == StateKnown || p.state == s {
		return
	}
	p.state = s
	p.setStateGaugesLocked()

	if s != StateNoData && !p.readyClosed {
		p.readyClosed = true
		close(p.ready)
	}
}

func (p *provider) setStateGaugesLocked() {
	for _, s := range allStates {
		v := 0.0
		if s == p.state {
			v = 1
		}
		p.metrics.StateGauge.WithLabelValues(s.String()).Set(v)
	}
}
