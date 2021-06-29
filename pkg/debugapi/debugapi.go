// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package debugapi exposes the debug API used to
// control and analyze low-level and runtime
// features and functionalities of Bee.
package debugapi

import (
	"crypto/ecdsa"
	"math/big"
	"net/http"
	"sync"

	"github.com/ethereum/go-ethereum/common"

	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/pkg/settlement"
	"github.com/ethersphere/bee/pkg/settlement/swap"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/lightnode"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/bee/pkg/traversal"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/semaphore"
)

// Service implements http.Handler interface to be used in HTTP server.
type Service struct {
	overlay            *swarm.Address
	publicKey          ecdsa.PublicKey
	pssPublicKey       ecdsa.PublicKey
	ethereumAddress    common.Address
	p2p                p2p.DebugService
	pingpong           pingpong.Interface
	topologyDriver     topology.Driver
	storer             storage.Storer
	tracer             *tracing.Tracer
	tags               *tags.Tags
	accounting         accounting.Interface
	pseudosettle       settlement.Interface
	chequebookEnabled  bool
	swapEnabled        bool
	chequebook         chequebook.Service
	swap               swap.Interface
	batchStore         postage.Storer
	transaction        transaction.Service
	post               postage.Service
	postageContract    postagecontract.Interface
	logger             logging.Logger
	corsAllowedOrigins []string
	metricsRegistry    *prometheus.Registry
	lightNodes         *lightnode.Container
	blockTime          *big.Int
	traverser          traversal.Traverser
	// handler is changed in the Configure method
	handler   http.Handler
	handlerMu sync.RWMutex

	// The following are semaphores which exists to limit concurrent access
	// to some parts of the resources in order to avoid undefined behaviour.
	postageSem       *semaphore.Weighted
	cashOutChequeSem *semaphore.Weighted
}

// New creates a new Debug API Service with only basic routers enabled in order
// to expose /addresses, /health endpoints, Go metrics and pprof. It is useful to expose
// these endpoints before all dependencies are configured and injected to have
// access to basic debugging tools and /health endpoint.
func New(publicKey, pssPublicKey ecdsa.PublicKey, ethereumAddress common.Address, logger logging.Logger, tracer *tracing.Tracer, corsAllowedOrigins []string, blockTime *big.Int, transaction transaction.Service) *Service {
	s := new(Service)
	s.publicKey = publicKey
	s.pssPublicKey = pssPublicKey
	s.ethereumAddress = ethereumAddress
	s.logger = logger
	s.tracer = tracer
	s.corsAllowedOrigins = corsAllowedOrigins
	s.blockTime = blockTime
	s.metricsRegistry = newMetricsRegistry()
	s.transaction = transaction
	s.postageSem = semaphore.NewWeighted(1)
	s.cashOutChequeSem = semaphore.NewWeighted(1)

	s.setRouter(s.newBasicRouter())

	return s
}

// Configure injects required dependencies and configuration parameters and
// constructs HTTP routes that depend on them. It is intended and safe to call
// this method only once.
func (s *Service) Configure(overlay swarm.Address, p2p p2p.DebugService, pingpong pingpong.Interface, topologyDriver topology.Driver, lightNodes *lightnode.Container, storer storage.Storer, tags *tags.Tags, accounting accounting.Interface, pseudosettle settlement.Interface, swapEnabled bool, chequebookEnabled bool, swap swap.Interface, chequebook chequebook.Service, batchStore postage.Storer, post postage.Service, postageContract postagecontract.Interface, traverser traversal.Traverser) {
	s.p2p = p2p
	s.pingpong = pingpong
	s.topologyDriver = topologyDriver
	s.storer = storer
	s.tags = tags
	s.accounting = accounting
	s.chequebookEnabled = chequebookEnabled
	s.chequebook = chequebook
	s.swapEnabled = swapEnabled
	s.swap = swap
	s.lightNodes = lightNodes
	s.batchStore = batchStore
	s.pseudosettle = pseudosettle
	s.overlay = &overlay
	s.post = post
	s.postageContract = postageContract
	s.traverser = traverser

	s.setRouter(s.newRouter())
}

// ServeHTTP implements http.Handler interface.
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// protect handler as it is changed by the Configure method
	s.handlerMu.RLock()
	h := s.handler
	s.handlerMu.RUnlock()

	h.ServeHTTP(w, r)
}
