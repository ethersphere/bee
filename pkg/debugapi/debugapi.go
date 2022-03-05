// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package debugapi exposes the debug API used to
// control and analyze low-level and runtime
// features and functionalities of Bee.
package debugapi

import (
	"context"
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

type authenticator interface {
	Authorize(string) bool
	GenerateKey(string, int) (string, error)
	Enforce(string, string, string) (bool, error)
}

// Service implements http.Handler interface to be used in HTTP server.
type Service struct {
	restricted         bool
	auth               authenticator
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
	beeMode            BeeNodeMode
	gatewayMode        bool
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
func New(publicKey, pssPublicKey ecdsa.PublicKey, ethereumAddress common.Address, logger logging.Logger, tracer *tracing.Tracer, corsAllowedOrigins []string, blockTime *big.Int, transaction transaction.Service, restrict bool, auth authenticator, gatewayMode bool, beeMode BeeNodeMode) *Service {
	s := new(Service)
	s.auth = auth
	s.restricted = restrict
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
	s.beeMode = beeMode
	s.gatewayMode = gatewayMode

	s.setRouter(s.newBasicRouter())

	return s
}

// Configure injects required dependencies and configuration parameters and
// constructs HTTP routes that depend on them. It is intended and safe to call
// this method only once.
func (s *Service) Configure(overlay swarm.Address, p2p p2p.DebugService, pingpong pingpong.Interface, topologyDriver topology.Driver, lightNodes *lightnode.Container, storer storage.Storer, tags *tags.Tags, accounting accounting.Interface, pseudosettle settlement.Interface, swapEnabled bool, chequebookEnabled bool, swap swap.Interface, chequebook chequebook.Service, batchStore postage.Storer, post postage.Service, postageContract postagecontract.Interface, traverser traversal.Traverser, chainEnabled bool) {
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
	if !chainEnabled {
		s.swap = new(noOpSwap)
	}
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

type noOpSwap struct {
}

func (*noOpSwap) TotalSent(peer swarm.Address) (totalSent *big.Int, err error) {
	return big.NewInt(0), nil
}

// TotalReceived returns the total amount received from a peer
func (*noOpSwap) TotalReceived(peer swarm.Address) (totalSent *big.Int, err error) {
	return big.NewInt(0), nil
}

// SettlementsSent returns sent settlements for each individual known peer
func (*noOpSwap) SettlementsSent() (map[string]*big.Int, error) {
	return nil, nil
}

// SettlementsReceived returns received settlements for each individual known peer
func (*noOpSwap) SettlementsReceived() (map[string]*big.Int, error) {
	return nil, nil
}

func (*noOpSwap) LastSentCheque(peer swarm.Address) (*chequebook.SignedCheque, error) {
	return nil, nil
}

// LastSentCheques returns the list of last sent cheques for all peers
func (*noOpSwap) LastSentCheques() (map[string]*chequebook.SignedCheque, error) {
	return nil, nil
}

// LastReceivedCheque returns the last received cheque for the peer
func (*noOpSwap) LastReceivedCheque(peer swarm.Address) (*chequebook.SignedCheque, error) {
	return nil, nil
}

// LastReceivedCheques returns the list of last received cheques for all peers
func (*noOpSwap) LastReceivedCheques() (map[string]*chequebook.SignedCheque, error) {
	return nil, nil
}

// CashCheque sends a cashing transaction for the last cheque of the peer
func (*noOpSwap) CashCheque(ctx context.Context, peer swarm.Address) (common.Hash, error) {
	return common.Hash{}, nil
}

// CashoutStatus gets the status of the latest cashout transaction for the peers chequebook
func (*noOpSwap) CashoutStatus(ctx context.Context, peer swarm.Address) (*chequebook.CashoutStatus, error) {
	return nil, nil
}
