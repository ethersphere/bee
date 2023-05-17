// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import "sync/atomic"

// ProbeStatus is the status of a probe.
// ProbeStatus is treated as a sync/atomic int32.
type ProbeStatus uint32

// String implements the fmt.Stringer interface.
func (ps ProbeStatus) String() string {
	switch ps {
	case ProbeStatusOK:
		return "ok"
	case ProbeStatusNOK:
		return "nok"
	}
	return "unknown"
}

const (
	// ProbeStatusOK indicates positive ProbeStatus status.
	ProbeStatusOK ProbeStatus = 1

	// ProbeStatusNOK indicates negative ProbeStatus status.
	ProbeStatusNOK ProbeStatus = 0
)

// Probe structure holds flags which indicate node healthiness (sometimes refert also as liveness) and readiness.
type Probe struct {
	// Healthy probe indicates if node, due to any reason, needs to restarted.
	healthy atomic.Uint32
	// Ready probe indicates that node is ready to start accepting traffic.
	ready atomic.Uint32
}

// NewProbe returns new Probe.
func NewProbe() *Probe {
	return &Probe{}
}

// Healthy returns the value of the healthy status.
func (p *Probe) Healthy() ProbeStatus {
	if p == nil {
		return ProbeStatusNOK
	}
	return ProbeStatus(p.healthy.Load())

}

// SetHealthy updates the value of the healthy status.
func (p *Probe) SetHealthy(ps ProbeStatus) {
	p.healthy.Store(uint32(ps))
}

// Ready returns the value of the ready status.
func (p *Probe) Ready() ProbeStatus {
	if p == nil {
		return ProbeStatusNOK
	}
	return ProbeStatus(p.ready.Load())
}

// SetReady updates the value of the ready status.
func (p *Probe) SetReady(ps ProbeStatus) {
	p.ready.Store(uint32(ps))
}
