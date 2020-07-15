// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/prometheus/client_golang/prometheus"
)

type Service interface {
	http.Handler
	MustRegisterMetrics(cs ...prometheus.Collector)
}

type server struct {
	Options
	http.Handler

	metricsRegistry *prometheus.Registry
}

type Options struct {
	Overlay        swarm.Address
	P2P            p2p.Service
	Pingpong       pingpong.Interface
	TopologyDriver topology.PeerAdder
	Storer         storage.Storer
	Logger         logging.Logger
	Tracer         *tracing.Tracer
	Tags           *tags.Tags
	Accounting     accounting.Interface
}

func New(o Options) Service {
	s := &server{
		Options:         o,
		metricsRegistry: newMetricsRegistry(),
	}

	s.setupRouting()

	return s
}
