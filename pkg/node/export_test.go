// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"context"
	"io"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/util/syncutil"
)

var (
	ValidatePublicAddress        = validatePublicAddress
	ValidateOptions              = validateOptions
	ValidateChainContractOptions = validateChainContractOptions
	ParsePaymentThreshold        = parsePaymentThreshold
	IsChainEnabled               = isChainEnabled
	BatchStoreExists             = batchStoreExists
	CheckOverlay                 = checkOverlay
	OverlayNonceExists           = overlayNonceExists
	SetOverlay                   = setOverlay
	SetupSwap                    = setupSwap
	DefaultSwapDeps              = defaultSwapDeps
	SetupPostageContract         = setupPostageContract
	DefaultPostageContractDeps   = defaultPostageContractDeps
	SetupSwapService             = setupSwapService
	DefaultSwapServiceDeps       = defaultSwapServiceDeps
)

type (
	SwapDeps              = swapDeps
	SwapResult            = swapResult
	PostageContractDeps   = postageContractDeps
	PostageContractResult = postageContractResult
	SwapServiceDeps       = swapServiceDeps
	SwapServiceResult     = swapServiceResult
)

const (
	OverlayNonceKey  = overlayNonce
	NoncedOverlayKey = noncedOverlayKey
)

// ShutdownTestClosers groups the io.Closer fields a Shutdown test wants to
// observe. Any nil entry leaves the corresponding *Bee field unset.
type ShutdownTestClosers struct {
	API                io.Closer
	PSS                io.Closer
	GSOC               io.Closer
	Pusher             io.Closer
	Puller             io.Closer
	Accounting         io.Closer
	PullSync           io.Closer
	Hive               io.Closer
	Salud              io.Closer
	P2P                io.Closer
	PriceOracle        io.Closer
	TransactionMonitor io.Closer
	Transaction        io.Closer
	Listener           io.Closer
	PostageService     io.Closer
	AccessControl      io.Closer
	Tracer             io.Closer
	Topology           io.Closer
	StorageIncentives  io.Closer
	Stabilization      io.Closer
	Localstore         io.Closer
	StateStore         io.Closer
	StamperStore       io.Closer
	Resolver           io.Closer
	EthClient          func()
}

// NewBeeForShutdownTest constructs a minimal *Bee suitable for exercising
// Shutdown without standing up any real subsystems. The returned context is
// the one Shutdown's ctxCancel cancels, so tests can assert it fires.
func NewBeeForShutdownTest(logger log.Logger, c ShutdownTestClosers) (*Bee, context.Context) {
	ctx, ctxCancel := context.WithCancel(context.Background())
	b := &Bee{
		logger:                   logger,
		ctxCancel:                ctxCancel,
		syncingStopped:           syncutil.NewSignaler(),
		apiCloser:                c.API,
		pssCloser:                c.PSS,
		gsocCloser:               c.GSOC,
		pusherCloser:             c.Pusher,
		pullerCloser:             c.Puller,
		accountingCloser:         c.Accounting,
		pullSyncCloser:           c.PullSync,
		hiveCloser:               c.Hive,
		saludCloser:              c.Salud,
		p2pService:               c.P2P,
		priceOracleCloser:        c.PriceOracle,
		transactionMonitorCloser: c.TransactionMonitor,
		transactionCloser:        c.Transaction,
		listenerCloser:           c.Listener,
		postageServiceCloser:     c.PostageService,
		accesscontrolCloser:      c.AccessControl,
		tracerCloser:             c.Tracer,
		topologyCloser:           c.Topology,
		storageIncetivesCloser:   c.StorageIncentives,
		stabilizationDetector:    c.Stabilization,
		localstoreCloser:         c.Localstore,
		stateStoreCloser:         c.StateStore,
		stamperStoreCloser:       c.StamperStore,
		resolverCloser:           c.Resolver,
		ethClientCloser:          c.EthClient,
	}
	return b, ctx
}
