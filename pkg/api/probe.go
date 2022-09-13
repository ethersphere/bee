// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import "sync/atomic"

// ProbeStatus is the status of a probe.
// ProbeStatus is treated as a sync/atomic int32.
type ProbeStatus int32

// get returns the value of the ProbeStatus.
func (ps *ProbeStatus) get() ProbeStatus {
	return ProbeStatus(atomic.LoadInt32((*int32)(ps)))
}

// set updates the value of the ProbeStatus.
func (ps *ProbeStatus) set(v ProbeStatus) {
	atomic.StoreInt32((*int32)(ps), int32(v))
}

// String implements the fmt.Stringer interface.
func (ps ProbeStatus) String() string {
	switch ps.get() {
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
	healthy ProbeStatus
	// Ready probe indicates that node is ready to start accepting traffic.
	ready ProbeStatus
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
	return p.healthy.get()

}

// SetHealthy updates the value of the healthy status.
func (p *Probe) SetHealthy(ps ProbeStatus) {
	p.healthy.set(ps)
}

// Ready returns the value of the ready status.
func (p *Probe) Ready() ProbeStatus {
	if p == nil {
		return ProbeStatusNOK
	}
	return p.ready.get()
}

// SetReady updates the value of the ready status.
func (p *Probe) SetReady(ps ProbeStatus) {
	p.ready.set(ps)
}
