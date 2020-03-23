// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/topology"
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
	P2P            p2p.Service
	Addressbook    addressbook.GetPutter
	TopologyDriver topology.Peerer
	Logger         logging.Logger
}

func New(o Options) Service {
	s := &server{
		Options:         o,
		metricsRegistry: newMetricsRegistry(),
	}

	s.setupRouting()

	return s
}
