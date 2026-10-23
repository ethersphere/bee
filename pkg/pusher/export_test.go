// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pusher

import (
	"time"

	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

const ParkDelay = parkDelay

// Parked exposes parked to tests.
type Parked struct{ p *parked }

func NewParked(delay, ttl time.Duration) *Parked { return &Parked{p: newParked(delay, ttl)} }

func (p *Parked) Park(idAddress swarm.Address, batchID []byte, now time.Time) bool {
	return p.p.park(idAddress, batchID, now)
}

func (p *Parked) Waiting(idAddress swarm.Address, batchID []byte, now time.Time) bool {
	return p.p.waiting(idAddress, batchID, now)
}

func (p *Parked) Forget(idAddress swarm.Address, batchID []byte) { p.p.forget(idAddress, batchID) }

func counterValue(c prometheus.Counter) float64 {
	m := &dto.Metric{}
	_ = c.Write(m)
	return m.GetCounter().GetValue()
}

func (s *Service) TotalParked() float64       { return counterValue(s.metrics.TotalParked) }
func (s *Service) TotalCouldNotSync() float64 { return counterValue(s.metrics.TotalCouldNotSync) }
func (s *Service) TotalErrors() float64       { return counterValue(s.metrics.TotalErrors) }
