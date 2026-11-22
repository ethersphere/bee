// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync

import (
	"github.com/ethersphere/bee/v2/pkg/pushsync/internal/firsthop"
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
	rad, err := ps.radius()
	if err != nil {
		return err
	}
	_, err = ps.checkReceipt(receipt, swarm.ZeroAddress, rad)
	return err
}

// FirstHops returns the first-hop memory.
func (ps *PushSync) FirstHops() *firsthop.Memory { return ps.firstHops }

func (ps *PushSync) FirstHopCount(source, result string) float64 {
	return counterValue(ps.metrics.FirstHop.WithLabelValues(source, result))
}

func counterValue(c prometheus.Counter) float64 {
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		return -1
	}
	return m.GetCounter().GetValue()
}
