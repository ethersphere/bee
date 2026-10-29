// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package depth

import (
	"context"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"
)

type Sample struct {
	Block    uint64
	TxIndex  uint
	LogIndex uint
	Depth    uint8
}

type UpdateResult = updateResult

const (
	WindowSize   = windowSize
	WindowRounds = windowRounds
	MinSamples   = minSamples

	UpdateNone       = updateNone
	UpdateFirst      = updateFirst
	UpdateStep       = updateStep
	UpdateNoMajority = updateNoMajority
	UpdateTooFew     = updateTooFew
)

var NewEstimator = newEstimator

func (e *estimator) Add(s Sample) bool {
	return e.add(sample{block: s.Block, txIndex: s.TxIndex, logIndex: s.LogIndex, depth: s.Depth})
}
func (e *estimator) Prune(head, blocksPerRound uint64) { e.prune(head, blocksPerRound) }
func (e *estimator) Update() UpdateResult              { return e.update() }
func (e *estimator) Depth() (uint8, bool)              { return e.depth() }
func (e *estimator) Size() int                         { return e.size() }
func (e *estimator) Blocks() []uint64 {
	b := make([]uint64, 0, len(e.samples))
	for _, s := range e.samples {
		b = append(b, s.block)
	}
	return b
}

func DecodeTruthSelected(a *abi.ABI, topic common.Hash, l types.Log) (Sample, error) {
	s, err := decodeTruthSelected(a, topic, l)
	if err != nil {
		return Sample{}, err
	}
	return Sample{Block: s.block, TxIndex: s.txIndex, LogIndex: s.logIndex, Depth: s.depth}, nil
}

func (p *provider) Backfill(ctx context.Context) error     { return p.backfill(ctx) }
func (p *provider) Poll(ctx context.Context) error         { return p.poll(ctx) }
func (p *provider) CheckHistory(ctx context.Context) error { return p.checkHistory(ctx) }

func (p *provider) WindowSamples() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.est.size()
}

func (p *provider) OldestSampleBlock() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.est.samples) == 0 {
		return 0
	}
	return p.est.samples[0].block
}

func (p *provider) RPCErrorsCounter() prometheus.Counter { return p.metrics.RPCErrors }

func (p *provider) NetworkGauge(source string) prometheus.Gauge {
	return p.metrics.Network.WithLabelValues(source)
}

func (p *provider) StateGauge(s State) prometheus.Gauge {
	return p.metrics.StateGauge.WithLabelValues(s.String())
}

func asProvider(pr Provider) *provider { return pr.(*provider) }

func Backfill(pr Provider, ctx context.Context) error     { return asProvider(pr).Backfill(ctx) }
func Poll(pr Provider, ctx context.Context) error         { return asProvider(pr).Poll(ctx) }
func CheckHistory(pr Provider, ctx context.Context) error { return asProvider(pr).CheckHistory(ctx) }
func WindowSamples(pr Provider) int                       { return asProvider(pr).WindowSamples() }
func OldestSampleBlock(pr Provider) uint64                { return asProvider(pr).OldestSampleBlock() }

func RPCErrorsCounter(pr Provider) prometheus.Counter { return asProvider(pr).RPCErrorsCounter() }

func NetworkGauge(pr Provider, source string) prometheus.Gauge {
	return asProvider(pr).NetworkGauge(source)
}
func StateGauge(pr Provider, s State) prometheus.Gauge { return asProvider(pr).StateGauge(s) }

// StopWorker stops the background worker without marking the provider closed, so Wait
// keeps its normal semantics while a test drives the provider synchronously.
func StopWorker(pr Provider) {
	p := asProvider(pr)
	p.cancel()
	<-p.done
}
