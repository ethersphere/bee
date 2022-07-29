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
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/auth"
	"github.com/ethersphere/bee/pkg/chainsync"
	"github.com/ethersphere/bee/pkg/chainsyncer"
	"github.com/ethersphere/bee/pkg/config"
	"github.com/ethersphere/bee/pkg/crypto"
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
	"github.com/ethersphere/bee/pkg/resolver/multiresolver"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle"
	"github.com/ethersphere/bee/pkg/settlement/swap"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/settlement/swap/erc20"
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
	"github.com/ethersphere/bee/pkg/util/ioutil"
	"github.com/hashicorp/go-multierror"
	ma "github.com/multiformats/go-multiaddr"
	"golang.org/x/crypto/sha3"
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
	errorLogWriter           io.Writer
	tracerCloser             io.Closer
	tagsCloser               io.Closer
	stateStoreCloser         io.Closer
	localstoreCloser         io.Closer
	nsCloser                 io.Closer
	topologyCloser           io.Closer
	topologyHalter           topology.Halter
	pusherCloser             io.Closer
	pullerCloser             io.Closer
	accountingCloser         io.Closer
	pullSyncCloser           io.Closer
	pssCloser                io.Closer
	ethClientCloser          func()
	transactionMonitorCloser io.Closer
	transactionCloser        io.Closer
	listenerCloser           io.Closer
	postageServiceCloser     io.Closer
	priceOracleCloser        io.Closer
	hiveCloser               io.Closer
	chainSyncerCloser        io.Closer
	shutdownInProgress       bool
	shutdownMutex            sync.Mutex
	syncingStopped           chan struct{}
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
	WelcomeMessage             string
	Bootnodes                  []string
	CORSAllowedOrigins         []string
	Logger                     logging.Logger
	TracingEnabled             bool
	TracingEndpoint            string
	TracingServiceName         string
	PaymentThreshold           string
	PaymentTolerance           int64
	PaymentEarly               int64
	ResolverConnectionCfgs     []multiresolver.ConnectionConfig
	RetrievalCaching           bool
	GatewayMode                bool
	BootnodeMode               bool
	SwapEndpoint               string
	SwapFactoryAddress         string
	SwapLegacyFactoryAddresses []string
	SwapInitialDeposit         string
	SwapEnable                 bool
	ChequebookEnable           bool
	FullNodeMode               bool
	Transaction                string
	BlockHash                  string
	PostageContractAddress     string
	PriceOracleAddress         string
	BlockTime                  uint64
	DeployGasPrice             string
	WarmupTime                 time.Duration
	ChainID                    int64
	Resync                     bool
	BlockProfile               bool
	MutexProfile               bool
	StaticNodes                []swarm.Address
	AllowPrivateCIDRs          bool
	Restricted                 bool
	TokenEncryptionKey         string
	AdminPasswordHash          string
	UsePostageSnapshot         bool
}

const (
	refreshRate                   = int64(4500000)
	lightRefreshRate              = int64(450000)
	basePrice                     = 10000
	postageSyncingStallingTimeout = 10 * time.Minute
	postageSyncingBackoffTimeout  = 5 * time.Second
	minPaymentThreshold           = 2 * refreshRate
	maxPaymentThreshold           = 24 * refreshRate
	mainnetNetworkID              = uint64(1)
)

var ErrInterruped = errors.New("interrupted")

func NewBee(interrupt chan os.Signal, addr string, publicKey *ecdsa.PublicKey, signer crypto.Signer, networkID uint64, logger logging.Logger, libp2pPrivateKey, pssPrivateKey *ecdsa.PrivateKey, o *Options) (b *Bee, err error) {

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

	// light nodes have zero warmup time for pull/pushsync protocols
	warmupTime := o.WarmupTime
	if !o.FullNodeMode {
		warmupTime = 0
	}

	sink := ioutil.WriterFunc(func(p []byte) (int, error) {
		logger.Error(string(p))
		return len(p), nil
	})

	b = &Bee{
		p2pCancel:      p2pCancel,
		errorLogWriter: sink,
		tracerCloser:   tracerCloser,
		syncingStopped: make(chan struct{}),
	}

	defer func(b *Bee) {
		if err != nil {
			logger.Errorf("got error %v, shutting down...", err)
			if err2 := b.Shutdown(); err2 != nil {
				logger.Errorf("got error while shutting down: %v", err2)
			}
		}
	}(b)

	stateStore, err := InitStateStore(logger, o.DataDir)
	if err != nil {
		return nil, err
	}
	b.stateStoreCloser = stateStore

	// Check if the the batchstore exists. If not, we can assume it's missing
	// due to a migration or it's a fresh install.
	batchStoreExists, err := batchStoreExists(stateStore)
	if err != nil {
		return nil, err
	}

	addressbook := addressbook.New(stateStore)

	var (
		chainBackend       transaction.Backend
		overlayEthAddress  common.Address
		chainID            int64
		transactionService transaction.Service
		transactionMonitor transaction.Monitor
		chequebookFactory  chequebook.Factory
		chequebookService  chequebook.Service = new(noOpChequebookService)
		chequeStore        chequebook.ChequeStore
		cashoutService     chequebook.CashoutService
		pollingInterval    = time.Duration(o.BlockTime) * time.Second
		erc20Service       erc20.Service
	)

	chainEnabled := isChainEnabled(o, o.SwapEndpoint, logger)

	var batchStore postage.Storer = new(postage.NoOpBatchStore)
	var unreserveFn func([]byte, uint8) (uint64, error)

	if chainEnabled {
		var evictFn = func(b []byte) error {
			_, err := unreserveFn(b, swarm.MaxPO+1)
			return err
		}
		batchStore, err = batchstore.New(stateStore, evictFn, logger)
		if err != nil {
			return nil, fmt.Errorf("batchstore: %w", err)
		}
	}

	chainBackend, overlayEthAddress, chainID, transactionMonitor, transactionService, err = InitChain(
		p2pCtx,
		logger,
		stateStore,
		o.SwapEndpoint,
		o.ChainID,
		signer,
		pollingInterval,
		chainEnabled)
	if err != nil {
		return nil, fmt.Errorf("init chain: %w", err)
	}
	b.ethClientCloser = chainBackend.Close

	if o.ChainID != -1 && o.ChainID != chainID {
		return nil, fmt.Errorf("connected to wrong ethereum network; network chainID %d; configured chainID %d", chainID, o.ChainID)
	}

	b.transactionCloser = tracerCloser
	b.transactionMonitorCloser = transactionMonitor

	var authenticator *auth.Authenticator

	if o.Restricted {
		if authenticator, err = auth.New(o.TokenEncryptionKey, o.AdminPasswordHash, logger); err != nil {
			return nil, fmt.Errorf("authenticator: %w", err)
		}
		logger.Info("starting with restricted APIs")
	}

	// set up basic debug api endpoints for debugging and /health endpoint
	beeNodeMode := api.LightMode
	if o.FullNodeMode {
		beeNodeMode = api.FullMode
	} else if !chainEnabled {
		beeNodeMode = api.UltraLightMode
	}

	var debugService *api.Service

	if o.DebugAPIAddr != "" {
		if o.MutexProfile {
			_ = runtime.SetMutexProfileFraction(1)
		}

		if o.BlockProfile {
			runtime.SetBlockProfileRate(1)
		}

		debugAPIListener, err := net.Listen("tcp", o.DebugAPIAddr)
		if err != nil {
			return nil, fmt.Errorf("debug api listener: %w", err)
		}

		debugService = api.New(*publicKey, pssPrivateKey.PublicKey, overlayEthAddress, logger, transactionService, batchStore, o.GatewayMode, beeNodeMode, o.ChequebookEnable, o.SwapEnable, o.CORSAllowedOrigins)
		debugService.MountTechnicalDebug()

		debugAPIServer := &http.Server{
			IdleTimeout:       30 * time.Second,
			ReadHeaderTimeout: 3 * time.Second,
			Handler:           debugService,
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

	var apiService *api.Service

	if o.Restricted {
		apiService = api.New(*publicKey, pssPrivateKey.PublicKey, overlayEthAddress, logger, transactionService, batchStore, o.GatewayMode, beeNodeMode, o.ChequebookEnable, o.SwapEnable, o.CORSAllowedOrigins)
		apiService.MountTechnicalDebug()

		apiServer := &http.Server{
			IdleTimeout:       30 * time.Second,
			ReadHeaderTimeout: 3 * time.Second,
			Handler:           apiService,
			ErrorLog:          log.New(b.errorLogWriter, "", 0),
		}

		apiListener, err := net.Listen("tcp", o.APIAddr)
		if err != nil {
			return nil, fmt.Errorf("api listener: %w", err)
		}

		go func() {
			logger.Infof("single debug & api address: %s", apiListener.Addr())

			if err := apiServer.Serve(apiListener); err != nil && err != http.ErrServerClosed {
				logger.Debugf("single debug & api server: %v", err)
				logger.Error("unable to serve debug & api")
			}
		}()

		b.apiServer = apiServer
		b.apiCloser = apiServer
	}

	// Sync the with the given Ethereum backend:
	isSynced, _, err := transaction.IsSynced(p2pCtx, chainBackend, maxDelay)
	if err != nil {
		return nil, fmt.Errorf("is synced: %w", err)
	}
	if !isSynced {
		logger.Infof("waiting to sync with the Ethereum backend")

		err := transaction.WaitSynced(p2pCtx, logger, chainBackend, maxDelay)
		if err != nil {
			return nil, fmt.Errorf("waiting backend sync: %w", err)
		}
	}

	if o.SwapEnable {
		chequebookFactory, err = InitChequebookFactory(
			logger,
			chainBackend,
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

		erc20Address, err := chequebookFactory.ERC20Address(p2pCtx)
		if err != nil {
			return nil, fmt.Errorf("factory fail: %w", err)
		}

		erc20Service = erc20.New(transactionService, erc20Address)

		if o.ChequebookEnable && chainEnabled {
			chequebookService, err = InitChequebookService(
				p2pCtx,
				logger,
				stateStore,
				signer,
				chainID,
				chainBackend,
				overlayEthAddress,
				transactionService,
				chequebookFactory,
				o.SwapInitialDeposit,
				o.DeployGasPrice,
				erc20Service,
			)
			if err != nil {
				return nil, err
			}
		}

		chequeStore, cashoutService = initChequeStoreCashout(
			stateStore,
			chainBackend,
			chequebookFactory,
			chainID,
			overlayEthAddress,
			transactionService,
		)
	}

	pubKey, _ := signer.PublicKey()
	if err != nil {
		return nil, err
	}

	var (
		blockHash []byte
		txHash    []byte
	)

	txHash, err = GetTxHash(stateStore, logger, o.Transaction)
	if err != nil {
		return nil, fmt.Errorf("invalid transaction hash: %w", err)
	}

	blockHash, err = GetTxNextBlock(p2pCtx, logger, chainBackend, transactionMonitor, pollingInterval, txHash, o.BlockHash)
	if err != nil {
		return nil, fmt.Errorf("invalid block hash: %w", err)
	}

	swarmAddress, err := crypto.NewOverlayAddress(*pubKey, networkID, blockHash)
	if err != nil {
		return nil, fmt.Errorf("compute overlay address: %w", err)
	}
	logger.Infof("using overlay address %s", swarmAddress)

	apiService.SetSwarmAddress(&swarmAddress)

	if err = CheckOverlayWithStore(swarmAddress, stateStore); err != nil {
		return nil, err
	}

	lightNodes := lightnode.NewContainer(swarmAddress)

	senderMatcher := transaction.NewMatcher(chainBackend, types.NewLondonSigner(big.NewInt(chainID)), stateStore, chainEnabled)
	_, err = senderMatcher.Matches(p2pCtx, txHash, networkID, swarmAddress, true)
	if err != nil {
		return nil, fmt.Errorf("identity transaction verification failed: %w", err)
	}

	var bootnodes []ma.Multiaddr

	for _, a := range o.Bootnodes {
		addr, err := ma.NewMultiaddr(a)
		if err != nil {
			logger.Debugf("multiaddress fail %s: %v", a, err)
			logger.Warningf("invalid bootnode address %s", a)
			continue
		}

		bootnodes = append(bootnodes, addr)
	}

	// Perform checks related to payment threshold calculations here to not duplicate
	// the checks in bootstrap process
	paymentThreshold, ok := new(big.Int).SetString(o.PaymentThreshold, 10)
	if !ok {
		return nil, fmt.Errorf("invalid payment threshold: %s", paymentThreshold)
	}

	if paymentThreshold.Cmp(big.NewInt(minPaymentThreshold)) < 0 {
		return nil, fmt.Errorf("payment threshold below minimum generally accepted value, need at least %d", minPaymentThreshold)
	}

	if paymentThreshold.Cmp(big.NewInt(maxPaymentThreshold)) > 0 {
		return nil, fmt.Errorf("payment threshold above maximum generally accepted value, needs to be reduced to at most %d", maxPaymentThreshold)
	}

	if o.PaymentTolerance < 0 {
		return nil, fmt.Errorf("invalid payment tolerance: %d", o.PaymentTolerance)
	}

	if o.PaymentEarly > 100 || o.PaymentEarly < 0 {
		return nil, fmt.Errorf("invalid payment early: %d", o.PaymentEarly)
	}

	var initBatchState *postage.ChainSnapshot
	// Bootstrap node with postage snapshot only if it is running on mainnet, is a fresh
	// install or explicitly asked by user to resync
	if networkID == mainnetNetworkID && o.UsePostageSnapshot && (!batchStoreExists || o.Resync) {
		start := time.Now()
		logger.Info("cold postage start detected. fetching postage stamp snapshot from swarm")
		initBatchState, err = bootstrapNode(
			addr,
			swarmAddress,
			txHash,
			chainID,
			overlayEthAddress,
			addressbook,
			bootnodes,
			lightNodes,
			senderMatcher,
			chequebookService,
			chequeStore,
			cashoutService,
			transactionService,
			stateStore,
			signer,
			networkID,
			logging.New(io.Discard, 0),
			libp2pPrivateKey,
			o,
		)
		logger.Infof("bootstrapper took %s", time.Since(start))
		if err != nil {
			logger.Errorf("bootstrapper failed to fetch batch state: %v", err)
		}
	}

	p2ps, err := libp2p.New(p2pCtx, signer, networkID, swarmAddress, addr, addressbook, stateStore, lightNodes, senderMatcher, logger, tracer, libp2p.Options{
		PrivateKey:      libp2pPrivateKey,
		NATAddr:         o.NATAddr,
		EnableWS:        o.EnableWS,
		WelcomeMessage:  o.WelcomeMessage,
		FullNode:        o.FullNodeMode,
		Transaction:     txHash,
		ValidateOverlay: chainEnabled,
	})
	if err != nil {
		return nil, fmt.Errorf("p2p service: %w", err)
	}

	apiService.SetP2P(p2ps)

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
		ReserveCapacity:        uint64(batchstore.Capacity),
		UnreserveFunc:          batchStore.Unreserve,
		OpenFilesLimit:         o.DBOpenFilesLimit,
		BlockCacheCapacity:     o.DBBlockCacheCapacity,
		WriteBufferSize:        o.DBWriteBufferSize,
		DisableSeeksCompaction: o.DBDisableSeeksCompaction,
	}

	storer, err := localstore.New(path, swarmAddress.Bytes(), stateStore, lo, logger)
	if err != nil {
		return nil, fmt.Errorf("localstore: %w", err)
	}
	b.localstoreCloser = storer
	unreserveFn = storer.UnreserveBatch

	validStamp := postage.ValidStamp(batchStore)
	post, err := postage.NewService(stateStore, batchStore, chainID)
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

	chainCfg, found := config.GetChainConfig(chainID)
	postageContractAddress, startBlock := chainCfg.PostageStamp, chainCfg.StartBlock
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

	eventListener = listener.New(b.syncingStopped, logger, chainBackend, postageContractAddress, o.BlockTime, postageSyncingStallingTimeout, postageSyncingBackoffTimeout)
	b.listenerCloser = eventListener

	batchSvc, err = batchservice.New(stateStore, batchStore, logger, eventListener, overlayEthAddress.Bytes(), post, sha3.New256, o.Resync)
	if err != nil {
		return nil, err
	}

	erc20Address, err := postagecontract.LookupERC20Address(p2pCtx, transactionService, postageContractAddress, chainEnabled)
	if err != nil {
		return nil, err
	}

	postageContractService = postagecontract.New(
		overlayEthAddress,
		postageContractAddress,
		erc20Address,
		transactionService,
		post,
		batchStore,
		chainEnabled,
	)

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

	// Construct protocols.
	pingPong := pingpong.New(p2ps, logger, tracer)

	if err = p2ps.AddProtocol(pingPong.Protocol()); err != nil {
		return nil, fmt.Errorf("pingpong service: %w", err)
	}

	hive, err := hive.New(p2ps, addressbook, networkID, o.BootnodeMode, o.AllowPrivateCIDRs, logger)
	if err != nil {
		return nil, fmt.Errorf("hive: %w", err)
	}

	if err = p2ps.AddProtocol(hive.Protocol()); err != nil {
		return nil, fmt.Errorf("hive service: %w", err)
	}
	b.hiveCloser = hive

	var swapService *swap.Service

	metricsDB, err := shed.NewDBWrap(stateStore.DB())
	if err != nil {
		return nil, fmt.Errorf("unable to create metrics storage for kademlia: %w", err)
	}

	kad, err := kademlia.New(swarmAddress, addressbook, hive, p2ps, pingPong, metricsDB, logger,
		kademlia.Options{Bootnodes: bootnodes, BootnodeMode: o.BootnodeMode, StaticNodes: o.StaticNodes, IgnoreRadius: !chainEnabled})
	if err != nil {
		return nil, fmt.Errorf("unable to create kademlia: %w", err)
	}
	b.topologyCloser = kad
	b.topologyHalter = kad
	hive.SetAddPeersHandler(kad.AddPeers)
	p2ps.SetPickyNotifier(kad)
	batchStore.SetRadiusSetter(kad)

	if batchSvc != nil && chainEnabled {
		syncedChan, err := batchSvc.Start(postageSyncStart, initBatchState)
		if err != nil {
			return nil, fmt.Errorf("unable to start batch service: %w", err)
		}
		// wait for the postage contract listener to sync
		logger.Info("waiting to sync postage contract data, this may take a while... more info available in Debug loglevel")

		// arguably this is not a very nice solution since we dont support
		// interrupts at this stage of the application lifecycle. some changes
		// would be needed on the cmd level to support context cancellation at
		// this stage
		select {
		case err = <-syncedChan:
			if err != nil {
				return nil, err
			}
		case <-interrupt:
			return nil, ErrInterruped
		}
	}

	pricer := pricer.NewFixedPricer(swarmAddress, basePrice)

	pricing := pricing.New(p2ps, logger, paymentThreshold, big.NewInt(minPaymentThreshold))

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

	acc, err := accounting.NewAccounting(
		paymentThreshold,
		o.PaymentTolerance,
		o.PaymentEarly,
		logger,
		stateStore,
		pricing,
		big.NewInt(refreshRate),
		p2ps,
	)
	if err != nil {
		return nil, fmt.Errorf("accounting: %w", err)
	}
	b.accountingCloser = acc

	var enforcedRefreshRate *big.Int

	if o.FullNodeMode {
		enforcedRefreshRate = big.NewInt(refreshRate)
	} else {
		enforcedRefreshRate = big.NewInt(lightRefreshRate)
	}

	pseudosettleService := pseudosettle.New(p2ps, logger, stateStore, acc, enforcedRefreshRate, big.NewInt(lightRefreshRate), p2ps)
	if err = p2ps.AddProtocol(pseudosettleService.Protocol()); err != nil {
		return nil, fmt.Errorf("pseudosettle service: %w", err)
	}

	acc.SetRefreshFunc(pseudosettleService.Pay)

	if o.SwapEnable && chainEnabled {
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

		if o.ChequebookEnable {
			acc.SetPayFunc(swapService.Pay)
		}
	}

	pricing.SetPaymentThresholdObserver(acc)

	retrieve := retrieval.New(swarmAddress, storer, p2ps, kad, logger, acc, pricer, tracer, o.RetrievalCaching, validStamp)
	tagService := tags.NewTags(stateStore, logger)
	b.tagsCloser = tagService

	pssService := pss.New(pssPrivateKey, logger)
	b.pssCloser = pssService

	var ns storage.Storer = netstore.New(storer, validStamp, retrieve, logger)
	b.nsCloser = ns

	traversalService := traversal.New(ns)

	pinningService := pinning.NewService(storer, stateStore, traversalService)

	pushSyncProtocol := pushsync.New(swarmAddress, blockHash, p2ps, storer, kad, tagService, o.FullNodeMode, pssService.TryUnwrap, validStamp, logger, acc, pricer, signer, tracer, warmupTime)

	// set the pushSyncer in the PSS
	pssService.SetPushSyncer(pushSyncProtocol)

	pusherService := pusher.New(networkID, storer, kad, pushSyncProtocol, validStamp, tagService, logger, tracer, warmupTime)
	b.pusherCloser = pusherService

	pullStorage := pullstorage.New(storer)

	pullSyncProtocol := pullsync.New(p2ps, pullStorage, pssService.TryUnwrap, validStamp, logger)
	b.pullSyncCloser = pullSyncProtocol

	var pullerService *puller.Puller
	if o.FullNodeMode && !o.BootnodeMode {
		pullerService = puller.New(stateStore, kad, pullSyncProtocol, logger, puller.Options{}, warmupTime)
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
		multiresolver.WithDefaultCIDResolver(),
	)
	b.resolverCloser = multiResolver
	var chainSyncer *chainsyncer.ChainSyncer

	if o.FullNodeMode {
		cs, err := chainsync.New(p2ps, chainBackend)
		if err != nil {
			return nil, fmt.Errorf("new chainsync: %w", err)
		}
		if err = p2ps.AddProtocol(cs.Protocol()); err != nil {
			return nil, fmt.Errorf("chainsync protocol: %w", err)
		}
		chainSyncer, err = chainsyncer.New(chainBackend, cs, kad, p2ps, logger, nil)
		if err != nil {
			return nil, fmt.Errorf("new chainsyncer: %w", err)
		}

		b.chainSyncerCloser = chainSyncer
	}

	feedFactory := factory.New(ns)
	steward := steward.New(storer, traversalService, retrieve, pushSyncProtocol)

	extraOpts := api.ExtraOptions{
		Pingpong:         pingPong,
		TopologyDriver:   kad,
		LightNodes:       lightNodes,
		Accounting:       acc,
		Pseudosettle:     pseudosettleService,
		Swap:             swapService,
		Chequebook:       chequebookService,
		BlockTime:        big.NewInt(int64(o.BlockTime)),
		Tags:             tagService,
		Storer:           ns,
		Resolver:         multiResolver,
		Pss:              pssService,
		TraversalService: traversalService,
		Pinning:          pinningService,
		FeedFactory:      feedFactory,
		Post:             post,
		PostageContract:  postageContractService,
		Steward:          steward,
	}

	if o.APIAddr != "" {
		if apiService == nil {
			apiService = api.New(*publicKey, pssPrivateKey.PublicKey, overlayEthAddress, logger, transactionService, batchStore, o.GatewayMode, beeNodeMode, o.ChequebookEnable, o.SwapEnable, o.CORSAllowedOrigins)
		}

		chunkC := apiService.Configure(signer, authenticator, tracer, api.Options{
			CORSAllowedOrigins: o.CORSAllowedOrigins,
			GatewayMode:        o.GatewayMode,
			WsPingPeriod:       60 * time.Second,
			Restricted:         o.Restricted,
		}, extraOpts, chainID, chainBackend, erc20Service)

		pusherService.AddFeed(chunkC)

		apiService.MountAPI()

		if !o.Restricted {
			apiServer := &http.Server{
				IdleTimeout:       30 * time.Second,
				ReadHeaderTimeout: 3 * time.Second,
				Handler:           apiService,
				ErrorLog:          log.New(b.errorLogWriter, "", 0),
			}

			apiListener, err := net.Listen("tcp", o.APIAddr)
			if err != nil {
				return nil, fmt.Errorf("api listener: %w", err)
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
		} else {
			// in Restricted mode we mount debug endpoints
			apiService.MountDebug(o.Restricted)
		}
	}

	if o.DebugAPIAddr != "" {
		// register metrics from components
		debugService.MustRegisterMetrics(p2ps.Metrics()...)
		debugService.MustRegisterMetrics(pingPong.Metrics()...)
		debugService.MustRegisterMetrics(acc.Metrics()...)
		debugService.MustRegisterMetrics(storer.Metrics()...)
		debugService.MustRegisterMetrics(kad.Metrics()...)

		if pullerService != nil {
			debugService.MustRegisterMetrics(pullerService.Metrics()...)
		}

		debugService.MustRegisterMetrics(pushSyncProtocol.Metrics()...)
		debugService.MustRegisterMetrics(pusherService.Metrics()...)
		debugService.MustRegisterMetrics(pullSyncProtocol.Metrics()...)
		debugService.MustRegisterMetrics(pullStorage.Metrics()...)
		debugService.MustRegisterMetrics(retrieve.Metrics()...)
		debugService.MustRegisterMetrics(lightNodes.Metrics()...)
		debugService.MustRegisterMetrics(hive.Metrics()...)

		if bs, ok := batchStore.(metrics.Collector); ok {
			debugService.MustRegisterMetrics(bs.Metrics()...)
		}
		if eventListener != nil {
			if ls, ok := eventListener.(metrics.Collector); ok {
				debugService.MustRegisterMetrics(ls.Metrics()...)
			}
		}
		if pssServiceMetrics, ok := pssService.(metrics.Collector); ok {
			debugService.MustRegisterMetrics(pssServiceMetrics.Metrics()...)
		}
		if swapBackendMetrics, ok := chainBackend.(metrics.Collector); ok {
			debugService.MustRegisterMetrics(swapBackendMetrics.Metrics()...)
		}
		if apiService != nil {
			debugService.MustRegisterMetrics(apiService.Metrics()...)
		}
		if l, ok := logger.(metrics.Collector); ok {
			debugService.MustRegisterMetrics(l.Metrics()...)
		}
		if nsMetrics, ok := ns.(metrics.Collector); ok {
			debugService.MustRegisterMetrics(nsMetrics.Metrics()...)
		}
		debugService.MustRegisterMetrics(pseudosettleService.Metrics()...)
		if swapService != nil {
			debugService.MustRegisterMetrics(swapService.Metrics()...)
		}
		if chainSyncer != nil {
			debugService.MustRegisterMetrics(chainSyncer.Metrics()...)
		}

		debugService.Configure(signer, authenticator, tracer, api.Options{
			CORSAllowedOrigins: o.CORSAllowedOrigins,
			GatewayMode:        o.GatewayMode,
			WsPingPeriod:       60 * time.Second,
			Restricted:         o.Restricted,
		}, extraOpts, chainID, chainBackend, erc20Service)

		debugService.SetP2P(p2ps)
		debugService.SetSwarmAddress(&swarmAddress)
		debugService.MountDebug(false)
	}

	if err := kad.Start(p2pCtx); err != nil {
		return nil, err
	}

	if err := p2ps.Ready(); err != nil {
		return nil, err
	}

	return b, nil
}

func (b *Bee) SyncingStopped() chan struct{} {
	return b.syncingStopped
}

func (b *Bee) Shutdown() error {
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
	if b.topologyHalter != nil {
		b.topologyHalter.Halt()
	}

	// halt p2p layer from accepting new connections
	// while shutting down other components
	if b.p2pHalter != nil {
		b.p2pHalter.Halt()
	}
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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

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

	var wg sync.WaitGroup
	wg.Add(7)
	go func() {
		defer wg.Done()
		tryClose(b.chainSyncerCloser, "chain syncer")
	}()
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
	go func() {
		defer wg.Done()
		tryClose(b.accountingCloser, "accounting")
	}()

	b.p2pCancel()
	go func() {
		defer wg.Done()
		tryClose(b.pullSyncCloser, "pull sync")
	}()
	go func() {
		defer wg.Done()
		tryClose(b.hiveCloser, "hive")
	}()

	wg.Wait()

	tryClose(b.p2pService, "p2p server")
	tryClose(b.priceOracleCloser, "price oracle service")

	wg.Add(3)
	go func() {
		defer wg.Done()
		tryClose(b.transactionMonitorCloser, "transaction monitor")
		tryClose(b.transactionCloser, "transaction")
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
	tryClose(b.nsCloser, "netstore")
	tryClose(b.stateStoreCloser, "statestore")
	tryClose(b.localstoreCloser, "localstore")
	tryClose(b.resolverCloser, "resolver service")

	return mErr
}

var ErrShutdownInProgress error = errors.New("shutdown in progress")

func isChainEnabled(o *Options, swapEndpoint string, logger logging.Logger) bool {
	chainDisabled := swapEndpoint == ""
	lightMode := !o.FullNodeMode

	if lightMode && chainDisabled { // ultra light mode is LightNode mode with chain disabled
		logger.Info("starting with a disabled chain backend")
		return false
	}

	logger.Info("starting with an enabled chain backend")
	return true // all other modes operate require chain enabled
}
