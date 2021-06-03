// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package node defines the concept of a Bee node
// by bootstrapping and injecting all necessary
// dependencies.
package node

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/feeds/factory"
	"github.com/ethersphere/bee/pkg/hive"
	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/metrics"
	"github.com/ethersphere/bee/pkg/netstore"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/pinning"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchservice"
	"github.com/ethersphere/bee/pkg/postage/batchstore"
	"github.com/ethersphere/bee/pkg/postage/listener"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/pkg/pricer"
	"github.com/ethersphere/bee/pkg/pricing"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/puller"
	"github.com/ethersphere/bee/pkg/pullsync"
	"github.com/ethersphere/bee/pkg/pullsync/pullstorage"
	"github.com/ethersphere/bee/pkg/pusher"
	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/recovery"
	"github.com/ethersphere/bee/pkg/resolver/multiresolver"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle"
	"github.com/ethersphere/bee/pkg/settlement/swap"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/settlement/swap/priceoracle"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/steward"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/kademlia"
	"github.com/ethersphere/bee/pkg/topology/lightnode"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/bee/pkg/traversal"
	"github.com/hashicorp/go-multierror"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

type Bee struct {
	p2pService               io.Closer
	p2pHalter                p2p.Halter
	p2pCancel                context.CancelFunc
	apiCloser                io.Closer
	apiServer                *http.Server
	debugAPIServer           *http.Server
	resolverCloser           io.Closer
	errorLogWriter           *io.PipeWriter
	tracerCloser             io.Closer
	tagsCloser               io.Closer
	stateStoreCloser         io.Closer
	localstoreCloser         io.Closer
	topologyCloser           io.Closer
	topologyHalter           topology.Halter
	pusherCloser             io.Closer
	pullerCloser             io.Closer
	pullSyncCloser           io.Closer
	pssCloser                io.Closer
	ethClientCloser          func()
	transactionMonitorCloser io.Closer
	recoveryHandleCleanup    func()
	listenerCloser           io.Closer
	postageServiceCloser     io.Closer
	priceOracleCloser        io.Closer
	shutdownInProgress       bool
	shutdownMutex            sync.Mutex
}

type Options struct {
	DataDir                    string
	CacheCapacity              uint64
	DBOpenFilesLimit           uint64
	DBWriteBufferSize          uint64
	DBBlockCacheCapacity       uint64
	DBDisableSeeksCompaction   bool
	APIAddr                    string
	DebugAPIAddr               string
	Addr                       string
	NATAddr                    string
	EnableWS                   bool
	EnableQUIC                 bool
	WelcomeMessage             string
	Bootnodes                  []string
	CORSAllowedOrigins         []string
	Logger                     logging.Logger
	Standalone                 bool
	TracingEnabled             bool
	TracingEndpoint            string
	TracingServiceName         string
	GlobalPinningEnabled       bool
	PaymentThreshold           string
	PaymentTolerance           string
	PaymentEarly               string
	ResolverConnectionCfgs     []multiresolver.ConnectionConfig
	GatewayMode                bool
	BootnodeMode               bool
	SwapEndpoint               string
	SwapFactoryAddress         string
	SwapLegacyFactoryAddresses []string
	SwapInitialDeposit         string
	SwapEnable                 bool
	FullNodeMode               bool
	Transaction                string
	PostageContractAddress     string
	PriceOracleAddress         string
	BlockTime                  uint64
	DeployGasPrice             string
}

const (
	refreshRate = int64(10000000)
	basePrice   = 10000
)

func NewBee(addr string, swarmAddress swarm.Address, publicKey ecdsa.PublicKey, signer crypto.Signer, networkID uint64, logger logging.Logger, libp2pPrivateKey, pssPrivateKey *ecdsa.PrivateKey, o Options) (b *Bee, err error) {
	tracer, tracerCloser, err := tracing.NewTracer(&tracing.Options{
		Enabled:     o.TracingEnabled,
		Endpoint:    o.TracingEndpoint,
		ServiceName: o.TracingServiceName,
	})
	if err != nil {
		return nil, fmt.Errorf("tracer: %w", err)
	}

	p2pCtx, p2pCancel := context.WithCancel(context.Background())
	defer func() {
		// if there's been an error on this function
		// we'd like to cancel the p2p context so that
		// incoming connections will not be possible
		if err != nil {
			p2pCancel()
		}
	}()

	b = &Bee{
		p2pCancel:      p2pCancel,
		errorLogWriter: logger.WriterLevel(logrus.ErrorLevel),
		tracerCloser:   tracerCloser,
	}

	var debugAPIService *debugapi.Service
	if o.DebugAPIAddr != "" {
		overlayEthAddress, err := signer.EthereumAddress()
		if err != nil {
			return nil, fmt.Errorf("eth address: %w", err)
		}
		// set up basic debug api endpoints for debugging and /health endpoint
		debugAPIService = debugapi.New(swarmAddress, publicKey, pssPrivateKey.PublicKey, overlayEthAddress, logger, tracer, o.CORSAllowedOrigins)

		debugAPIListener, err := net.Listen("tcp", o.DebugAPIAddr)
		if err != nil {
			return nil, fmt.Errorf("debug api listener: %w", err)
		}

		debugAPIServer := &http.Server{
			IdleTimeout:       30 * time.Second,
			ReadHeaderTimeout: 3 * time.Second,
			Handler:           debugAPIService,
			ErrorLog:          log.New(b.errorLogWriter, "", 0),
		}

		go func() {
			logger.Infof("debug api address: %s", debugAPIListener.Addr())

			if err := debugAPIServer.Serve(debugAPIListener); err != nil && err != http.ErrServerClosed {
				logger.Debugf("debug api server: %v", err)
				logger.Error("unable to serve debug api")
			}
		}()

		b.debugAPIServer = debugAPIServer
	}

	stateStore, err := InitStateStore(logger, o.DataDir)
	if err != nil {
		return nil, err
	}
	b.stateStoreCloser = stateStore

	err = CheckOverlayWithStore(swarmAddress, stateStore)
	if err != nil {
		return nil, err
	}

	addressbook := addressbook.New(stateStore)

	var (
		swapBackend        *ethclient.Client
		overlayEthAddress  common.Address
		chainID            int64
		transactionService transaction.Service
		transactionMonitor transaction.Monitor
		chequebookFactory  chequebook.Factory
		chequebookService  chequebook.Service
		chequeStore        chequebook.ChequeStore
		cashoutService     chequebook.CashoutService
	)
	if !o.Standalone {
		swapBackend, overlayEthAddress, chainID, transactionMonitor, transactionService, err = InitChain(
			p2pCtx,
			logger,
			stateStore,
			o.SwapEndpoint,
			signer,
			o.BlockTime,
		)
		if err != nil {
			return nil, fmt.Errorf("init chain: %w", err)
		}
		b.ethClientCloser = swapBackend.Close
		b.transactionMonitorCloser = transactionMonitor
	}

	if o.SwapEnable {
		chequebookFactory, err = InitChequebookFactory(
			logger,
			swapBackend,
			chainID,
			transactionService,
			o.SwapFactoryAddress,
			o.SwapLegacyFactoryAddresses,
		)
		if err != nil {
			return nil, err
		}

		if err = chequebookFactory.VerifyBytecode(p2pCtx); err != nil {
			return nil, fmt.Errorf("factory fail: %w", err)
		}

		chequebookService, err = InitChequebookService(
			p2pCtx,
			logger,
			stateStore,
			signer,
			chainID,
			swapBackend,
			overlayEthAddress,
			transactionService,
			chequebookFactory,
			o.SwapInitialDeposit,
			o.DeployGasPrice,
		)
		if err != nil {
			return nil, err
		}

		chequeStore, cashoutService = initChequeStoreCashout(
			stateStore,
			swapBackend,
			chequebookFactory,
			chainID,
			overlayEthAddress,
			transactionService,
		)
	}

	lightNodes := lightnode.NewContainer(swarmAddress)

	txHash, err := getTxHash(stateStore, logger, o)
	if err != nil {
		return nil, fmt.Errorf("invalid transaction hash: %w", err)
	}

	senderMatcher := transaction.NewMatcher(swapBackend, types.NewEIP155Signer(big.NewInt(chainID)))

	p2ps, err := libp2p.New(p2pCtx, signer, networkID, swarmAddress, addr, addressbook, stateStore, lightNodes, senderMatcher, logger, tracer, libp2p.Options{
		PrivateKey:     libp2pPrivateKey,
		NATAddr:        o.NATAddr,
		EnableWS:       o.EnableWS,
		EnableQUIC:     o.EnableQUIC,
		Standalone:     o.Standalone,
		WelcomeMessage: o.WelcomeMessage,
		FullNode:       o.FullNodeMode,
		Transaction:    txHash,
	})
	if err != nil {
		return nil, fmt.Errorf("p2p service: %w", err)
	}
	b.p2pService = p2ps
	b.p2pHalter = p2ps

	// localstore depends on batchstore
	var path string

	if o.DataDir != "" {
		logger.Infof("using datadir in: '%s'", o.DataDir)
		path = filepath.Join(o.DataDir, "localstore")
	}
	lo := &localstore.Options{
		Capacity:               o.CacheCapacity,
		OpenFilesLimit:         o.DBOpenFilesLimit,
		BlockCacheCapacity:     o.DBBlockCacheCapacity,
		WriteBufferSize:        o.DBWriteBufferSize,
		DisableSeeksCompaction: o.DBDisableSeeksCompaction,
	}

	storer, err := localstore.New(path, swarmAddress.Bytes(), lo, logger)
	if err != nil {
		return nil, fmt.Errorf("localstore: %w", err)
	}
	b.localstoreCloser = storer

	batchStore, err := batchstore.New(stateStore, storer.UnreserveBatch)
	if err != nil {
		return nil, fmt.Errorf("batchstore: %w", err)
	}
	validStamp := postage.ValidStamp(batchStore)
	post, err := postage.NewService(stateStore, chainID)
	if err != nil {
		return nil, fmt.Errorf("postage service load: %w", err)
	}
	b.postageServiceCloser = post

	var (
		postageContractService postagecontract.Interface
		batchSvc               postage.EventUpdater
		eventListener          postage.Listener
	)

	var postageSyncStart uint64 = 0
	if !o.Standalone {
		postageContractAddress, startBlock, found := listener.DiscoverAddresses(chainID)
		if o.PostageContractAddress != "" {
			if !common.IsHexAddress(o.PostageContractAddress) {
				return nil, errors.New("malformed postage stamp address")
			}
			postageContractAddress = common.HexToAddress(o.PostageContractAddress)
		} else if !found {
			return nil, errors.New("no known postage stamp addresses for this network")
		}
		if found {
			postageSyncStart = startBlock
		}

		eventListener = listener.New(logger, swapBackend, postageContractAddress, o.BlockTime, &pidKiller{node: b})
		b.listenerCloser = eventListener

		batchSvc = batchservice.New(stateStore, batchStore, logger, eventListener)

		erc20Address, err := postagecontract.LookupERC20Address(p2pCtx, transactionService, postageContractAddress)
		if err != nil {
			return nil, err
		}

		postageContractService = postagecontract.New(
			overlayEthAddress,
			postageContractAddress,
			erc20Address,
			transactionService,
			post,
		)
	}

	if !o.Standalone {
		if natManager := p2ps.NATManager(); natManager != nil {
			// wait for nat manager to init
			logger.Debug("initializing NAT manager")
			select {
			case <-natManager.Ready():
				// this is magic sleep to give NAT time to sync the mappings
				// this is a hack, kind of alchemy and should be improved
				time.Sleep(3 * time.Second)
				logger.Debug("NAT manager initialized")
			case <-time.After(10 * time.Second):
				logger.Warning("NAT manager init timeout")
			}
		}
	}

	// Construct protocols.
	pingPong := pingpong.New(p2ps, logger, tracer)

	if err = p2ps.AddProtocol(pingPong.Protocol()); err != nil {
		return nil, fmt.Errorf("pingpong service: %w", err)
	}

	hive := hive.New(p2ps, addressbook, networkID, logger)
	if err = p2ps.AddProtocol(hive.Protocol()); err != nil {
		return nil, fmt.Errorf("hive service: %w", err)
	}

	var bootnodes []ma.Multiaddr
	if o.Standalone {
		logger.Info("Starting node in standalone mode, no p2p connections will be made or accepted")
	} else {
		for _, a := range o.Bootnodes {
			addr, err := ma.NewMultiaddr(a)
			if err != nil {
				logger.Debugf("multiaddress fail %s: %v", a, err)
				logger.Warningf("invalid bootnode address %s", a)
				continue
			}

			bootnodes = append(bootnodes, addr)
		}
	}

	var swapService *swap.Service

	metricsDB, err := shed.NewDBWrap(stateStore.DB())
	if err != nil {
		return nil, fmt.Errorf("unable to create metrics storage for kademlia: %w", err)
	}

	kad := kademlia.New(swarmAddress, addressbook, hive, p2ps, metricsDB, logger, kademlia.Options{Bootnodes: bootnodes, StandaloneMode: o.Standalone, BootnodeMode: o.BootnodeMode})
	b.topologyCloser = kad
	b.topologyHalter = kad
	hive.SetAddPeersHandler(kad.AddPeers)
	p2ps.SetPickyNotifier(kad)
	batchStore.SetRadiusSetter(kad)

	if batchSvc != nil {
		syncedChan, err := batchSvc.Start(postageSyncStart)
		if err != nil {
			return nil, fmt.Errorf("unable to start batch service: %w", err)
		}
		// wait for the postage contract listener to sync
		logger.Info("waiting to sync postage contract data, this may take a while... more info available in Debug loglevel")

		// arguably this is not a very nice solution since we dont support
		// interrupts at this stage of the application lifecycle. some changes
		// would be needed on the cmd level to support context cancellation at
		// this stage
		<-syncedChan

	}
	paymentThreshold, ok := new(big.Int).SetString(o.PaymentThreshold, 10)
	if !ok {
		return nil, fmt.Errorf("invalid payment threshold: %s", paymentThreshold)
	}

	pricer := pricer.NewFixedPricer(swarmAddress, basePrice)

	minThreshold := pricer.MostExpensive()

	pricing := pricing.New(p2ps, logger, paymentThreshold, minThreshold)

	if err = p2ps.AddProtocol(pricing.Protocol()); err != nil {
		return nil, fmt.Errorf("pricing service: %w", err)
	}

	addrs, err := p2ps.Addresses()
	if err != nil {
		return nil, fmt.Errorf("get server addresses: %w", err)
	}

	for _, addr := range addrs {
		logger.Debugf("p2p address: %s", addr)
	}

	paymentTolerance, ok := new(big.Int).SetString(o.PaymentTolerance, 10)
	if !ok {
		return nil, fmt.Errorf("invalid payment tolerance: %s", paymentTolerance)
	}
	paymentEarly, ok := new(big.Int).SetString(o.PaymentEarly, 10)
	if !ok {
		return nil, fmt.Errorf("invalid payment early: %s", paymentEarly)
	}

	acc, err := accounting.NewAccounting(
		paymentThreshold,
		paymentTolerance,
		paymentEarly,
		logger,
		stateStore,
		pricing,
		big.NewInt(refreshRate),
	)
	if err != nil {
		return nil, fmt.Errorf("accounting: %w", err)
	}

	pseudosettleService := pseudosettle.New(p2ps, logger, stateStore, acc, big.NewInt(refreshRate), p2ps)
	if err = p2ps.AddProtocol(pseudosettleService.Protocol()); err != nil {
		return nil, fmt.Errorf("pseudosettle service: %w", err)
	}

	acc.SetRefreshFunc(pseudosettleService.Pay)

	if o.SwapEnable {
		var priceOracle priceoracle.Service
		swapService, priceOracle, err = InitSwap(
			p2ps,
			logger,
			stateStore,
			networkID,
			overlayEthAddress,
			chequebookService,
			chequeStore,
			cashoutService,
			acc,
			o.PriceOracleAddress,
			chainID,
			transactionService,
		)
		if err != nil {
			return nil, err
		}
		b.priceOracleCloser = priceOracle
		acc.SetPayFunc(swapService.Pay)
	}

	pricing.SetPaymentThresholdObserver(acc)

	retrieve := retrieval.New(swarmAddress, storer, p2ps, kad, logger, acc, pricer, tracer)
	tagService := tags.NewTags(stateStore, logger)
	b.tagsCloser = tagService

	pssService := pss.New(pssPrivateKey, logger)
	b.pssCloser = pssService

	var ns storage.Storer
	if o.GlobalPinningEnabled {
		// create recovery callback for content repair
		recoverFunc := recovery.NewCallback(pssService)
		ns = netstore.New(storer, validStamp, recoverFunc, retrieve, logger)
	} else {
		ns = netstore.New(storer, validStamp, nil, retrieve, logger)
	}

	traversalService := traversal.New(ns)

	pinningService := pinning.NewService(storer, stateStore, traversalService)

	pushSyncProtocol := pushsync.New(swarmAddress, p2ps, storer, kad, tagService, o.FullNodeMode, pssService.TryUnwrap, validStamp, logger, acc, pricer, signer, tracer)

	// set the pushSyncer in the PSS
	pssService.SetPushSyncer(pushSyncProtocol)

	if o.GlobalPinningEnabled {
		// register function for chunk repair upon receiving a trojan message
		chunkRepairHandler := recovery.NewRepairHandler(ns, logger, pushSyncProtocol)
		b.recoveryHandleCleanup = pssService.Register(recovery.Topic, chunkRepairHandler)
	}

	pusherService := pusher.New(networkID, storer, kad, pushSyncProtocol, tagService, logger, tracer)
	b.pusherCloser = pusherService

	pullStorage := pullstorage.New(storer)

	pullSyncProtocol := pullsync.New(p2ps, pullStorage, pssService.TryUnwrap, validStamp, logger)
	b.pullSyncCloser = pullSyncProtocol

	var pullerService *puller.Puller
	if o.FullNodeMode {
		pullerService := puller.New(stateStore, kad, pullSyncProtocol, logger, puller.Options{})
		b.pullerCloser = pullerService
	}

	retrieveProtocolSpec := retrieve.Protocol()
	pushSyncProtocolSpec := pushSyncProtocol.Protocol()
	pullSyncProtocolSpec := pullSyncProtocol.Protocol()

	if o.FullNodeMode {
		logger.Info("starting in full mode")
	} else {
		logger.Info("starting in light mode")
		p2p.WithBlocklistStreams(p2p.DefaultBlocklistTime, retrieveProtocolSpec)
		p2p.WithBlocklistStreams(p2p.DefaultBlocklistTime, pushSyncProtocolSpec)
		p2p.WithBlocklistStreams(p2p.DefaultBlocklistTime, pullSyncProtocolSpec)
	}

	if err = p2ps.AddProtocol(retrieveProtocolSpec); err != nil {
		return nil, fmt.Errorf("retrieval service: %w", err)
	}
	if err = p2ps.AddProtocol(pushSyncProtocolSpec); err != nil {
		return nil, fmt.Errorf("pushsync service: %w", err)
	}
	if err = p2ps.AddProtocol(pullSyncProtocolSpec); err != nil {
		return nil, fmt.Errorf("pullsync protocol: %w", err)
	}

	multiResolver := multiresolver.NewMultiResolver(
		multiresolver.WithConnectionConfigs(o.ResolverConnectionCfgs),
		multiresolver.WithLogger(o.Logger),
	)
	b.resolverCloser = multiResolver

	var apiService api.Service
	if o.APIAddr != "" {
		// API server
		feedFactory := factory.New(ns)
		steward := steward.New(storer, traversalService, pushSyncProtocol)
		apiService = api.New(tagService, ns, multiResolver, pssService, traversalService, pinningService, feedFactory, post, postageContractService, steward, signer, logger, tracer, api.Options{
			CORSAllowedOrigins: o.CORSAllowedOrigins,
			GatewayMode:        o.GatewayMode,
			WsPingPeriod:       60 * time.Second,
		})
		apiListener, err := net.Listen("tcp", o.APIAddr)
		if err != nil {
			return nil, fmt.Errorf("api listener: %w", err)
		}

		apiServer := &http.Server{
			IdleTimeout:       30 * time.Second,
			ReadHeaderTimeout: 3 * time.Second,
			Handler:           apiService,
			ErrorLog:          log.New(b.errorLogWriter, "", 0),
		}

		go func() {
			logger.Infof("api address: %s", apiListener.Addr())

			if err := apiServer.Serve(apiListener); err != nil && err != http.ErrServerClosed {
				logger.Debugf("api server: %v", err)
				logger.Error("unable to serve api")
			}
		}()

		b.apiServer = apiServer
		b.apiCloser = apiService
	}

	if debugAPIService != nil {
		// register metrics from components
		debugAPIService.MustRegisterMetrics(p2ps.Metrics()...)
		debugAPIService.MustRegisterMetrics(pingPong.Metrics()...)
		debugAPIService.MustRegisterMetrics(acc.Metrics()...)
		debugAPIService.MustRegisterMetrics(storer.Metrics()...)
		debugAPIService.MustRegisterMetrics(kad.Metrics()...)

		if pullerService != nil {
			debugAPIService.MustRegisterMetrics(pullerService.Metrics()...)
		}

		debugAPIService.MustRegisterMetrics(pushSyncProtocol.Metrics()...)
		debugAPIService.MustRegisterMetrics(pusherService.Metrics()...)
		debugAPIService.MustRegisterMetrics(pullSyncProtocol.Metrics()...)
		debugAPIService.MustRegisterMetrics(pullStorage.Metrics()...)
		debugAPIService.MustRegisterMetrics(retrieve.Metrics()...)
		debugAPIService.MustRegisterMetrics(lightNodes.Metrics()...)

		if bs, ok := batchStore.(metrics.Collector); ok {
			debugAPIService.MustRegisterMetrics(bs.Metrics()...)
		}

		if eventListener != nil {
			if ls, ok := eventListener.(metrics.Collector); ok {
				debugAPIService.MustRegisterMetrics(ls.Metrics()...)
			}
		}

		if pssServiceMetrics, ok := pssService.(metrics.Collector); ok {
			debugAPIService.MustRegisterMetrics(pssServiceMetrics.Metrics()...)
		}

		if apiService != nil {
			debugAPIService.MustRegisterMetrics(apiService.Metrics()...)
		}
		if l, ok := logger.(metrics.Collector); ok {
			debugAPIService.MustRegisterMetrics(l.Metrics()...)
		}

		debugAPIService.MustRegisterMetrics(pseudosettleService.Metrics()...)

		if swapService != nil {
			debugAPIService.MustRegisterMetrics(swapService.Metrics()...)
		}

		// inject dependencies and configure full debug api http path routes
		debugAPIService.Configure(p2ps, pingPong, kad, lightNodes, storer, tagService, acc, pseudosettleService, o.SwapEnable, swapService, chequebookService, batchStore)
	}

	if err := kad.Start(p2pCtx); err != nil {
		return nil, err
	}
	p2ps.Ready()

	return b, nil
}

func (b *Bee) Shutdown(ctx context.Context) error {
	var mErr error

	// if a shutdown is already in process, return here
	b.shutdownMutex.Lock()
	if b.shutdownInProgress {
		b.shutdownMutex.Unlock()
		return ErrShutdownInProgress
	}
	b.shutdownInProgress = true
	b.shutdownMutex.Unlock()

	// halt kademlia while shutting down other
	// components.
	b.topologyHalter.Halt()

	// halt p2p layer from accepting new connections
	// while shutting down other components
	b.p2pHalter.Halt()
	// tryClose is a convenient closure which decrease
	// repetitive io.Closer tryClose procedure.
	tryClose := func(c io.Closer, errMsg string) {
		if c == nil {
			return
		}
		if err := c.Close(); err != nil {
			mErr = multierror.Append(mErr, fmt.Errorf("%s: %w", errMsg, err))
		}
	}

	tryClose(b.apiCloser, "api")

	var eg errgroup.Group
	if b.apiServer != nil {
		eg.Go(func() error {
			if err := b.apiServer.Shutdown(ctx); err != nil {
				return fmt.Errorf("api server: %w", err)
			}
			return nil
		})
	}
	if b.debugAPIServer != nil {
		eg.Go(func() error {
			if err := b.debugAPIServer.Shutdown(ctx); err != nil {
				return fmt.Errorf("debug api server: %w", err)
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		mErr = multierror.Append(mErr, err)
	}

	if b.recoveryHandleCleanup != nil {
		b.recoveryHandleCleanup()
	}
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		tryClose(b.pssCloser, "pss")
	}()
	go func() {
		defer wg.Done()
		tryClose(b.pusherCloser, "pusher")
	}()
	go func() {
		defer wg.Done()
		tryClose(b.pullerCloser, "puller")
	}()

	b.p2pCancel()
	go func() {
		defer wg.Done()
		tryClose(b.pullSyncCloser, "pull sync")
	}()

	wg.Wait()

	tryClose(b.p2pService, "p2p server")
	tryClose(b.priceOracleCloser, "price oracle service")

	wg.Add(3)
	go func() {
		defer wg.Done()
		tryClose(b.transactionMonitorCloser, "transaction monitor")
	}()
	go func() {
		defer wg.Done()
		tryClose(b.listenerCloser, "listener")
	}()
	go func() {
		defer wg.Done()
		tryClose(b.postageServiceCloser, "postage service")
	}()

	wg.Wait()

	if c := b.ethClientCloser; c != nil {
		c()
	}

	tryClose(b.tracerCloser, "tracer")
	tryClose(b.tagsCloser, "tag persistence")
	tryClose(b.topologyCloser, "topology driver")
	tryClose(b.stateStoreCloser, "statestore")
	tryClose(b.localstoreCloser, "localstore")
	tryClose(b.errorLogWriter, "error log writer")
	tryClose(b.resolverCloser, "resolver service")

	return mErr
}

func getTxHash(stateStore storage.StateStorer, logger logging.Logger, o Options) ([]byte, error) {
	if o.Standalone {
		return nil, nil // in standalone mode tx hash is not used
	}

	if o.Transaction != "" {
		txHashTrimmed := strings.TrimPrefix(o.Transaction, "0x")
		if len(txHashTrimmed) != 64 {
			return nil, errors.New("invalid length")
		}
		txHash, err := hex.DecodeString(txHashTrimmed)
		if err != nil {
			return nil, err
		}
		logger.Infof("using the provided transaction hash %x", txHash)
		return txHash, nil
	}

	var txHash common.Hash
	key := chequebook.ChequebookDeploymentKey
	if err := stateStore.Get(key, &txHash); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, errors.New("chequebook deployment transaction hash not found. Please specify the transaction hash manually.")
		}
		return nil, err
	}

	logger.Infof("using the chequebook transaction hash %x", txHash)
	return txHash.Bytes(), nil
}

// pidKiller is used to issue a forced shut down of the node from sub modules. The issue with using the
// node's Shutdown method is that it only shuts down the node and does not exit the start process
// which is waiting on the os.Signals. This is not desirable, but currently bee node cannot handle
// rate-limiting blockchain API calls properly. We will shut down the node in this case to allow the
// user to rectify the API issues (by adjusting limits or using a different one). There is no platform
// agnostic way to trigger os.Signals in go unfortunately. Which is why we will use the process.Kill
// approach which works on windows as well.
type pidKiller struct {
	node *Bee
}

var ErrShutdownInProgress error = errors.New("shutdown in progress")

func (p *pidKiller) Shutdown(ctx context.Context) error {
	err := p.node.Shutdown(ctx)
	if err != nil {
		return err
	}
	ps, err := os.FindProcess(syscall.Getpid())
	if err != nil {
		return err
	}
	return ps.Kill()
}
