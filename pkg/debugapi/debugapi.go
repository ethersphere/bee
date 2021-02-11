// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package debugapi exposes the debug API used to
// control and analyze low-level features and functionalities
// of Bee.
package debugapi

import (
	"crypto/ecdsa"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/settlement"
	"github.com/ethersphere/bee/pkg/settlement/swap"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
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
	Overlay           swarm.Address
	PublicKey         ecdsa.PublicKey
	PSSPublicKey      ecdsa.PublicKey
	EthereumAddress   common.Address
	P2P               p2p.DebugService
	Pingpong          pingpong.Interface
	TopologyDriver    topology.Driver
	Storer            storage.Storer
	Logger            logging.Logger
	Tracer            *tracing.Tracer
	Tags              *tags.Tags
	Accounting        accounting.Interface
	Settlement        settlement.Interface
	ChequebookEnabled bool
	Chequebook        chequebook.Service
	Swap              swap.ApiInterface
	metricsRegistry   *prometheus.Registry
	http.Handler
}

func New(overlay swarm.Address, publicKey, pssPublicKey ecdsa.PublicKey, ethereumAddress common.Address, p2p p2p.DebugService, pingpong pingpong.Interface, topologyDriver topology.Driver, storer storage.Storer, logger logging.Logger, tracer *tracing.Tracer, tags *tags.Tags, accounting accounting.Interface, settlement settlement.Interface, chequebookEnabled bool, swap swap.ApiInterface, chequebook chequebook.Service) Service {
	s := &server{
		Overlay:           overlay,
		PublicKey:         publicKey,
		PSSPublicKey:      pssPublicKey,
		EthereumAddress:   ethereumAddress,
		P2P:               p2p,
		Pingpong:          pingpong,
		TopologyDriver:    topologyDriver,
		Storer:            storer,
		Logger:            logger,
		Tracer:            tracer,
		Tags:              tags,
		Accounting:        accounting,
		Settlement:        settlement,
		metricsRegistry:   newMetricsRegistry(),
		ChequebookEnabled: chequebookEnabled,
		Chequebook:        chequebook,
		Swap:              swap,
	}

	s.setupRouting()

	return s
}
