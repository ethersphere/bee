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
func (m *hopMemory) MarkProbe(rad uint8, chunk swarm.Address)    { m.markProbe(rad, chunk) }
func (m *hopMemory) Len() int                                    { return m.len() }

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
func (ps *PushSync) HopRecords() int { return ps.hops.len() }

func (ps *PushSync) FirstHopCount(source, result string) float64 {
	return counterValue(ps.metrics.FirstHop.WithLabelValues(source, result))
}

func (ps *PushSync) NeighborhoodKnownShallowCount() float64 {
	return counterValue(ps.metrics.NeighborhoodKnownShallow)
}

func counterValue(c prometheus.Counter) float64 {
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		return -1
	}
	return m.GetCounter().GetValue()
}

// SetNoHopMemory makes the origin pick the first hop without the first-hop memory.
func (ps *PushSync) SetNoHopMemory(v bool) { ps.noHopMemory = v }

// SetLegacyForwarder makes the forwarder act as in v2.8.3.
func (ps *PushSync) SetLegacyForwarder(v bool) { ps.legacyForwarder = v }
