// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync

import (
	"time"

	"github.com/ethersphere/bee/v2/pkg/pushsync/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

var (
	ProtocolName    = protocolName
	ProtocolVersion = protocolVersion
	StreamName      = streamName
)

// CheckReceipt exposes the unexported receipt verification (signature recovery,
// overlay derivation, proximity and shallow-receipt logic) for fuzzing.
func (ps *PushSync) CheckReceipt(receipt *pb.Receipt) error {
	_, err := ps.checkReceipt(receipt, swarm.ZeroAddress)
	return err
}

type HopMemory = hopMemory

func NewHopMemory(now func() time.Time) *HopMemory { return newHopMemory(now) }

func (m *hopMemory) RecordValid(rad uint8, chunk, peer swarm.Address) {
	m.recordValid(rad, chunk, peer)
}

func (m *hopMemory) RecordShallow(rad uint8, chunk, peer swarm.Address) {
	m.recordShallow(rad, chunk, peer)
}
func (m *hopMemory) Forget(rad uint8, chunk, peer swarm.Address) { m.forget(rad, chunk, peer) }
func (m *hopMemory) ClaimProbe(rad uint8, chunk swarm.Address) bool {
	return m.claimProbe(rad, chunk)
}

// Len returns the number of records.
func (m *hopMemory) Len() int {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	var n int
	for _, h := range m.hoods {
		n += len(h.peers)
	}
	return n
}

// Hoods returns the number of neighborhoods.
func (m *hopMemory) Hoods() int {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	return len(m.hoods)
}

func (m *hopMemory) View(rad uint8, chunk swarm.Address) (valid, blocked []swarm.Address, probeDue bool) {
	return m.view(rad, chunk)
}

// SetHopClock replaces the clock of the first-hop memory.
func (ps *PushSync) SetHopClock(now func() time.Time) {
	ps.hops.mtx.Lock()
	defer ps.hops.mtx.Unlock()
	ps.hops.now = now
}

// HopRecords returns the number of first-hop records.
func (ps *PushSync) HopRecords() int { return ps.hops.Len() }

func (ps *PushSync) FirstHopCount(source, result string) float64 {
	return counterValue(ps.metrics.FirstHop.WithLabelValues(source, result))
}

// Hops returns the first-hop memory.
func (ps *PushSync) Hops() *HopMemory { return ps.hops }

func counterValue(c prometheus.Counter) float64 {
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		return -1
	}
	return m.GetCounter().GetValue()
}
