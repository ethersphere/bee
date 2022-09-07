package api

import "sync/atomic"

const (
	probeStatusPending int32 = iota
	ProbeStatusOK
	ProbeStatusNOK
)

// Probe structure holds flags which indicate node healthiness (k8s equivalent for liveness) and readiness.
type Probe struct {
	// Healthy probe indicates node overall health.
	healthy int32
	// Ready probe indicates that node is ready to start accepting traffic.
	ready int32
}

func NewProbe() *Probe {
	return &Probe{
		healthy: probeStatusPending,
		ready:   probeStatusPending,
	}
}

func (p *Probe) Healthy() bool {
	return atomic.LoadInt32(&p.healthy) == ProbeStatusOK
}

func (p *Probe) SetHealthy(s int32) {
	atomic.StoreInt32(&p.healthy, s)
}

func (p *Probe) Ready() bool {
	return atomic.LoadInt32(&p.ready) == ProbeStatusOK
}

func (p *Probe) SetReady(s int32) {
	atomic.StoreInt32(&p.ready, s)
}

func (s *Service) SetProbe(probe *Probe) {
	s.probe = probe
}
