package api

import (
	"net/http"

	"github.com/janos/bee/pkg/p2p"
	"github.com/janos/bee/pkg/pingpong"
	"github.com/prometheus/client_golang/prometheus"
)

type Service interface {
	http.Handler
	Metrics() (cs []prometheus.Collector)
}

type server struct {
	Options
	http.Handler
	metrics metrics
}

type Options struct {
	P2P      p2p.Service
	Pingpong *pingpong.Service
}

func New(o Options) Service {
	s := &server{
		Options: o,
		metrics: newMetrics(),
	}

	s.setupRouting()

	return s
}
