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
	stdlog "log"
	"math/big"
	"net"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
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
	"github.com/ethersphere/bee/pkg/log"
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
	"github.com/ethersphere/bee/pkg/storageincentives"
	"github.com/ethersphere/bee/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/pkg/storageincentives/staking"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/depthmonitor"
	"github.com/ethersphere/bee/pkg/topology/kademlia"
	"github.com/ethersphere/bee/pkg/topology/lightnode"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/bee/pkg/traversal"
	"github.com/ethersphere/bee/pkg/util"
	"github.com/ethersphere/bee/pkg/util/ioutil"
	"github.com/hashicorp/go-multierror"
	ma "github.com/multiformats/go-multiaddr"
	"golang.org/x/crypto/sha3"
	"golang.org/x/sync/errgroup"
)

// LoggerName is the tree path name of the logger for this package.
const LoggerName = "node"

type Bee struct {
	p2pService               io.Closer
	p2pHalter                p2p.Halter
	ctxCancel                context.CancelFunc
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
	depthMonitorCloser       io.Closer
	storageIncetivesCloser   io.Closer
	shutdownInProgress       bool
	shutdownMutex            sync.Mutex
	syncingStopped           *util.Signaler
}

type Options struct {
	DataDir                       string
	CacheCapacity                 uint64
	DBOpenFilesLimit              uint64
	DBWriteBufferSize             uint64
	DBBlockCacheCapacity          uint64
	DBDisableSeeksCompaction      bool
	APIAddr                       string
	DebugAPIAddr                  string
	Addr                          string
	NATAddr                       string
	EnableWS                      bool
	WelcomeMessage                string
	Bootnodes                     []string
	CORSAllowedOrigins            []string
	Logger                        log.Logger
	TracingEnabled                bool
	TracingEndpoint               string
	TracingServiceName            string
	PaymentThreshold              string
	PaymentTolerance              int64
	PaymentEarly                  int64
	ResolverConnectionCfgs        []multiresolver.ConnectionConfig
	RetrievalCaching              bool
	BootnodeMode                  bool
	BlockchainRpcEndpoint         string
	SwapFactoryAddress            string
	SwapLegacyFactoryAddresses    []string
	SwapInitialDeposit            string
	SwapEnable                    bool
	ChequebookEnable              bool
	FullNodeMode                  bool
	Transaction                   string
	BlockHash                     string
	PostageContractAddress        string
	PostageContractStartBlock     uint64
	StakingContractAddress        string
	PriceOracleAddress            string
	RedistributionContractAddress string
	BlockTime                     time.Duration
	DeployGasPrice                string
	WarmupTime                    time.Duration
	ChainID                       int64
	Resync                        bool
	BlockProfile                  bool
	MutexProfile                  bool
	StaticNodes                   []swarm.Address
	AllowPrivateCIDRs             bool
	Restricted                    bool
	TokenEncryptionKey            string
	AdminPasswordHash             string
	UsePostageSnapshot            bool
	EnableStorageIncentives       bool
}

const (
	refreshRate                   = int64(4500000)            // accounting units refreshed per second
	lightFactor                   = 10                        // downscale payment thresholds and their change rate, and refresh rates by this for light nodes
	lightRefreshRate              = refreshRate / lightFactor // refresh rate used by / for light nodes
	basePrice                     = 10000                     // minimal price for retrieval and pushsync requests of maximum proximity
	postageSyncingStallingTimeout = 10 * time.Minute          //
	postageSyncingBackoffTimeout  = 5 * time.Second           //
	minPaymentThreshold           = 2 * refreshRate           // minimal accepted payment threshold of full nodes
	maxPaymentThreshold           = 24 * refreshRate          // maximal accepted payment threshold of full nodes
	mainnetNetworkID              = uint64(1)                 //
)

func NewBee(ctx context.Context, addr string, publicKey *ecdsa.PublicKey, signer crypto.Signer, networkID uint64, logger log.Logger, libp2pPrivateKey, pssPrivateKey *ecdsa.PrivateKey, o *Options) (b *Bee, err error) {
	tracer, tracerCloser, err := tracing.NewTracer(&tracing.Options{
		Enabled:     o.TracingEnabled,
		Endpoint:    o.TracingEndpoint,
		ServiceName: o.TracingServiceName,
	})
	if err != nil {
		return nil, fmt.Errorf("tracer: %w", err)
	}

	ctx, ctxCancel := context.WithCancel(ctx)
	defer func() {
		// if there's been an error on this function
		// we'd like to cancel the p2p context so that
		// incoming connections will not be possible
		if err != nil {
			ctxCancel()
		}
	}()

	// light nodes have zero warmup time for pull/pushsync protocols
	warmupTime := o.WarmupTime
	if !o.FullNodeMode {
		warmupTime = 0
	}

	sink := ioutil.WriterFunc(func(p []byte) (int, error) {
		logger.Error(nil, string(p))
		return len(p), nil
	})

	b = &Bee{
		ctxCancel:      ctxCancel,
		errorLogWriter: sink,
		tracerCloser:   tracerCloser,
		syncingStopped: util.NewSignaler(),
	}

	defer func(b *Bee) {
		if err != nil {
			logger.Error(err, "got error, shutting down...")
			if err2 := b.Shutdown(); err2 != nil {
				logger.Error(err2, "got error while shutting down")
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
		return nil, fmt.Errorf("batchstore: exists: %w", err)
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
		erc20Service       erc20.Service
	)

	chainEnabled := isChainEnabled(o, o.BlockchainRpcEndpoint, logger)

	var batchStore postage.Storer = new(postage.NoOpBatchStore)
	var evictFn func([]byte) error

	if chainEnabled {
		batchStore, err = batchstore.New(
			stateStore,
			func(id []byte) error {
				return evictFn(id)
			},
			logger,
		)
		if err != nil {
			return nil, fmt.Errorf("batchstore: %w", err)
		}
	}

	chainBackend, overlayEthAddress, chainID, transactionMonitor, transactionService, err = InitChain(
		ctx,
		logger,
		stateStore,
		o.BlockchainRpcEndpoint,
		o.ChainID,
		signer,
		o.BlockTime,
		chainEnabled)
	if err != nil {
		return nil, fmt.Errorf("init chain: %w", err)
	}
	b.ethClientCloser = chainBackend.Close

	logger.Info("using chain with network network", "chain_id", chainID, "network_id", networkID)

	if o.ChainID != -1 && o.ChainID != chainID {
		return nil, fmt.Errorf("connected to wrong ethereum network; network chainID %d; configured chainID %d", chainID, o.ChainID)
	}

	b.transactionCloser = tracerCloser
	b.transactionMonitorCloser = transactionMonitor

	var authenticator auth.Authenticator

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

	// Create api.Probe in healthy state and switch to ready state after all components have been constructed
	probe := api.NewProbe()
	probe.SetHealthy(api.ProbeStatusOK)
	defer func(probe *api.Probe) {
		if err != nil {
			probe.SetHealthy(api.ProbeStatusNOK)
		} else {
			probe.SetReady(api.ProbeStatusOK)
		}
	}(probe)

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

		debugService = api.New(*publicKey, pssPrivateKey.PublicKey, overlayEthAddress, logger, transactionService, batchStore, beeNodeMode, o.ChequebookEnable, o.SwapEnable, chainBackend, o.CORSAllowedOrigins)
		debugService.MountTechnicalDebug()
		debugService.SetProbe(probe)

		debugAPIServer := &http.Server{
			IdleTimeout:       30 * time.Second,
			ReadHeaderTimeout: 3 * time.Second,
			Handler:           debugService,
			ErrorLog:          stdlog.New(b.errorLogWriter, "", 0),
		}

		go func() {
			logger.Info("starting debug server", "address", debugAPIListener.Addr())

			if err := debugAPIServer.Serve(debugAPIListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Debug("debug api server failed to start", "error", err)
				logger.Error(nil, "debug api server failed to start")
			}
		}()

		b.debugAPIServer = debugAPIServer
	}

	var apiService *api.Service

	if o.Restricted {
		apiService = api.New(*publicKey, pssPrivateKey.PublicKey, overlayEthAddress, logger, transactionService, batchStore, beeNodeMode, o.ChequebookEnable, o.SwapEnable, chainBackend, o.CORSAllowedOrigins)
		apiService.MountTechnicalDebug()
		apiService.SetProbe(probe)

		apiServer := &http.Server{
			IdleTimeout:       30 * time.Second,
			ReadHeaderTimeout: 3 * time.Second,
			Handler:           apiService,
			ErrorLog:          stdlog.New(b.errorLogWriter, "", 0),
		}

		apiListener, err := net.Listen("tcp", o.APIAddr)
		if err != nil {
			return nil, fmt.Errorf("api listener: %w", err)
		}

		go func() {
			logger.Info("starting debug & api server", "address", apiListener.Addr())

			if err := apiServer.Serve(apiListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Debug("debug & api server failed to start", "error", err)
				logger.Error(nil, "debug & api server failed to start")
			}
		}()

		b.apiServer = apiServer
		b.apiCloser = apiServer
	}

	// Sync the with the given Ethereum backend:
	isSynced, _, err := transaction.IsSynced(ctx, chainBackend, maxDelay)
	if err != nil {
		return nil, fmt.Errorf("is synced: %w", err)
	}
	if !isSynced {
		logger.Info("waiting to sync with the Ethereum backend")

		err := transaction.WaitSynced(ctx, logger, chainBackend, maxDelay)
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

		if err = chequebookFactory.VerifyBytecode(ctx); err != nil {
			return nil, fmt.Errorf("factory fail: %w", err)
		}

		erc20Address, err := chequebookFactory.ERC20Address(ctx)
		if err != nil {
			return nil, fmt.Errorf("factory fail: %w", err)
		}

		erc20Service = erc20.New(transactionService, erc20Address)

		if o.ChequebookEnable && chainEnabled {
			chequebookService, err = InitChequebookService(
				ctx,
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

	// if theres a previous transaction hash, and not a new chequebook deployment on a node starting from scratch
	// get old overlay
	// mine nonce that gives similar new overlay
	nonce, nonceExists, err := overlayNonceExists(stateStore)
	if err != nil {
		return nil, fmt.Errorf("check presence of nonce: %w", err)
	}

	swarmAddress, err := crypto.NewOverlayAddress(*pubKey, networkID, nonce)
	if err != nil {
		return nil, fmt.Errorf("compute overlay address: %w", err)
	}
	logger.Info("using overlay address", "address", swarmAddress)

	if !nonceExists {
		err := setOverlayNonce(stateStore, nonce)
		if err != nil {
			return nil, fmt.Errorf("statestore: save new overlay nonce: %w", err)
		}

		err = SetOverlayInStore(swarmAddress, stateStore)
		if err != nil {
			return nil, fmt.Errorf("statestore: save new overlay: %w", err)
		}
	}

	apiService.SetSwarmAddress(&swarmAddress)

	if err = CheckOverlayWithStore(swarmAddress, stateStore); err != nil {
		return nil, err
	}

	lightNodes := lightnode.NewContainer(swarmAddress)

	bootnodes := make([]ma.Multiaddr, 0, len(o.Bootnodes))

	for _, a := range o.Bootnodes {
		addr, err := ma.NewMultiaddr(a)
		if err != nil {
			logger.Debug("create bootnode multiaddress from string failed", "string", a, "error", err)
			logger.Warning("create bootnode multiaddress from string failed", "string", a)
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
			ctx,
			addr,
			swarmAddress,
			nonce,
			chainID,
			overlayEthAddress,
			addressbook,
			bootnodes,
			lightNodes,
			chequebookService,
			chequeStore,
			cashoutService,
			transactionService,
			stateStore,
			signer,
			networkID,
			log.Noop,
			libp2pPrivateKey,
			o,
		)
		logger.Info("bootstrapper created", "elapsed", time.Since(start))
		if err != nil {
			logger.Error(err, "bootstrapper failed to fetch batch state")
		}
	}

	p2ps, err := libp2p.New(ctx, signer, networkID, swarmAddress, addr, addressbook, stateStore, lightNodes, logger, tracer, libp2p.Options{
		PrivateKey:      libp2pPrivateKey,
		NATAddr:         o.NATAddr,
		EnableWS:        o.EnableWS,
		WelcomeMessage:  o.WelcomeMessage,
		FullNode:        o.FullNodeMode,
		Nonce:           nonce,
		ValidateOverlay: chainEnabled,
		Registry:        debugService.MetricsRegistry(),
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
		logger.Info("using datadir", "path", o.DataDir)
		path = filepath.Join(o.DataDir, "localstore")
	}

	validStamp := postage.ValidStamp(batchStore)

	lo := &localstore.Options{
		Capacity:               o.CacheCapacity,
		ReserveCapacity:        uint64(batchstore.Capacity),
		UnreserveFunc:          batchStore.Unreserve,
		OpenFilesLimit:         o.DBOpenFilesLimit,
		BlockCacheCapacity:     o.DBBlockCacheCapacity,
		WriteBufferSize:        o.DBWriteBufferSize,
		DisableSeeksCompaction: o.DBDisableSeeksCompaction,
		ValidStamp:             validStamp,
	}

	storer, err := localstore.New(path, swarmAddress.Bytes(), stateStore, lo, logger)
	if err != nil {
		return nil, fmt.Errorf("localstore: %w", err)
	}
	b.localstoreCloser = storer
	evictFn = storer.EvictBatch

	post, err := postage.NewService(stateStore, batchStore, chainID)
	if err != nil {
		return nil, fmt.Errorf("postage service load: %w", err)
	}
	b.postageServiceCloser = post
	batchStore.SetBatchExpiryHandler(post)

	var (
		postageStampContractService postagecontract.Interface
		batchSvc                    postage.EventUpdater
		eventListener               postage.Listener
	)

	chainCfg, found := config.GetByChainID(chainID)
	postageStampContractAddress, postageSyncStart := chainCfg.PostageStampAddress, chainCfg.PostageStampStartBlock
	if o.PostageContractAddress != "" {
		if !common.IsHexAddress(o.PostageContractAddress) {
			return nil, errors.New("malformed postage stamp address")
		}
		postageStampContractAddress = common.HexToAddress(o.PostageContractAddress)
		if o.PostageContractStartBlock == 0 {
			return nil, errors.New("postage contract start block option not provided")
		}
		postageSyncStart = o.PostageContractStartBlock
	} else if !found {
		return nil, errors.New("no known postage stamp addresses for this network")
	}

	postageStampContractABI, err := abi.JSON(strings.NewReader(chainCfg.PostageStampABI))
	if err != nil {
		return nil, fmt.Errorf("unable to parse postage stamp ABI: %w", err)
	}

	bzzTokenAddress, err := postagecontract.LookupERC20Address(ctx, transactionService, postageStampContractAddress, postageStampContractABI, chainEnabled)
	if err != nil {
		return nil, err
	}

	postageStampContractService = postagecontract.New(
		overlayEthAddress,
		postageStampContractAddress,
		postageStampContractABI,
		bzzTokenAddress,
		transactionService,
		post,
		batchStore,
		chainEnabled,
	)

	eventListener = listener.New(b.syncingStopped, logger, chainBackend, postageStampContractAddress, postageStampContractABI, o.BlockTime, postageSyncingStallingTimeout, postageSyncingBackoffTimeout)
	b.listenerCloser = eventListener

	batchSvc, err = batchservice.New(stateStore, batchStore, logger, eventListener, overlayEthAddress.Bytes(), post, sha3.New256, o.Resync)
	if err != nil {
		return nil, err
	}

	if natManager := p2ps.NATManager(); natManager != nil {
		// wait for nat manager to init
		logger.Debug("initializing NAT manager")
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-natManager.Ready():
			// this is magic sleep to give NAT time to sync the mappings
			// this is a hack, kind of alchemy and should be improved
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(3 * time.Second):
			}
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

	var (
		syncErr    atomic.Value
		syncStatus atomic.Value

		syncStatusFn = func() (isDone bool, err error) {
			iErr := syncErr.Load()
			if iErr != nil {
				err = iErr.(error)
			}
			isDone = syncStatus.Load() != nil
			return isDone, err
		}
	)

	if batchSvc != nil && chainEnabled {
		logger.Info("waiting to sync postage contract data, this may take a while... more info available in Debug loglevel")
		if o.FullNodeMode {
			err = batchSvc.Start(ctx, postageSyncStart, initBatchState)
			syncStatus.Store(true)
			if err != nil {
				syncErr.Store(err)
				return nil, fmt.Errorf("unable to start batch service: %w", err)
			} else {
				err = post.SetExpired()
				if err != nil {
					return nil, fmt.Errorf("unable to set expirations: %w", err)
				}
			}
		} else {
			go func() {
				logger.Info("started postage contract data sync in the background...")
				err := batchSvc.Start(ctx, postageSyncStart, initBatchState)
				syncStatus.Store(true)
				if err != nil {
					syncErr.Store(err)
					logger.Error(err, "unable to sync batches")
					b.syncingStopped.Signal() // trigger shutdown in start.go
				} else {
					err = post.SetExpired()
					if err != nil {
						logger.Error(err, "unable to set expirations")
					}
				}
			}()
		}
	}

	minThreshold := big.NewInt(2 * refreshRate)
	maxThreshold := big.NewInt(24 * refreshRate)

	if !o.FullNodeMode {
		minThreshold = big.NewInt(2 * lightRefreshRate)
	}

	lightPaymentThreshold := new(big.Int).Div(paymentThreshold, big.NewInt(lightFactor))

	pricer := pricer.NewFixedPricer(swarmAddress, basePrice)

	if paymentThreshold.Cmp(minThreshold) < 0 {
		return nil, fmt.Errorf("payment threshold below minimum generally accepted value, need at least %s", minThreshold)
	}

	if paymentThreshold.Cmp(maxThreshold) > 0 {
		return nil, fmt.Errorf("payment threshold above maximum generally accepted value, needs to be reduced to at most %s", maxThreshold)
	}

	pricing := pricing.New(p2ps, logger, paymentThreshold, lightPaymentThreshold, minThreshold)

	if err = p2ps.AddProtocol(pricing.Protocol()); err != nil {
		return nil, fmt.Errorf("pricing service: %w", err)
	}

	addrs, err := p2ps.Addresses()
	if err != nil {
		return nil, fmt.Errorf("get server addresses: %w", err)
	}

	for _, addr := range addrs {
		logger.Debug("p2p address", "address", addr)
	}

	var enforcedRefreshRate *big.Int

	if o.FullNodeMode {
		enforcedRefreshRate = big.NewInt(refreshRate)
	} else {
		enforcedRefreshRate = big.NewInt(lightRefreshRate)
	}

	acc, err := accounting.NewAccounting(
		paymentThreshold,
		o.PaymentTolerance,
		o.PaymentEarly,
		logger,
		stateStore,
		pricing,
		new(big.Int).Set(enforcedRefreshRate),
		lightFactor,
		p2ps,
	)
	if err != nil {
		return nil, fmt.Errorf("accounting: %w", err)
	}
	b.accountingCloser = acc

	pseudosettleService := pseudosettle.New(p2ps, logger, stateStore, acc, new(big.Int).Set(enforcedRefreshRate), big.NewInt(lightRefreshRate), p2ps)
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

	ns := netstore.New(storer, validStamp, retrieve, logger)
	b.nsCloser = ns

	traversalService := traversal.New(ns)

	pinningService := pinning.NewService(storer, stateStore, traversalService)

	pushSyncProtocol := pushsync.New(swarmAddress, nonce, p2ps, storer, kad, tagService, o.FullNodeMode, pssService.TryUnwrap, validStamp, logger, acc, pricer, signer, tracer, warmupTime)

	// set the pushSyncer in the PSS
	pssService.SetPushSyncer(pushSyncProtocol)

	pusherService := pusher.New(networkID, storer, kad, pushSyncProtocol, validStamp, tagService, logger, tracer, warmupTime, pusher.DefaultRetryCount)
	b.pusherCloser = pusherService

	pullStorage := pullstorage.New(storer, logger)

	pullSyncProtocol := pullsync.New(p2ps, pullStorage, pssService.TryUnwrap, validStamp, logger)
	b.pullSyncCloser = pullSyncProtocol

	var pullerService *puller.Puller
	if o.FullNodeMode && !o.BootnodeMode {
		pullerService = puller.New(stateStore, kad, batchStore, pullSyncProtocol, logger, puller.Options{SyncSleepDur: puller.DefaultSyncErrorSleepDur}, warmupTime)
		b.pullerCloser = pullerService
	}

	retrieveProtocolSpec := retrieve.Protocol()
	pushSyncProtocolSpec := pushSyncProtocol.Protocol()
	pullSyncProtocolSpec := pullSyncProtocol.Protocol()

	if o.FullNodeMode && !o.BootnodeMode {
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

	stakingContractAddress := chainCfg.StakingAddress
	if o.StakingContractAddress != "" {
		if !common.IsHexAddress(o.StakingContractAddress) {
			return nil, errors.New("malformed staking contract address")
		}
		stakingContractAddress = common.HexToAddress(o.StakingContractAddress)
	}

	stakingContractABI, err := abi.JSON(strings.NewReader(chainCfg.StakingABI))
	if err != nil {
		return nil, fmt.Errorf("unable to parse staking ABI: %w", err)
	}

	stakingContract := staking.New(swarmAddress, overlayEthAddress, stakingContractAddress, stakingContractABI, bzzTokenAddress, transactionService, common.BytesToHash(nonce))

	var agent *storageincentives.Agent
	if o.FullNodeMode {

		depthMonitor := depthmonitor.New(kad, pullerService, storer, batchStore, logger, warmupTime, depthmonitor.DefaultWakeupInterval)
		b.depthMonitorCloser = depthMonitor

		if !o.BootnodeMode && o.EnableStorageIncentives {

			redistributionContractAddress := chainCfg.RedistributionAddress
			if o.RedistributionContractAddress != "" {
				if !common.IsHexAddress(o.RedistributionContractAddress) {
					return nil, errors.New("malformed redistribution contract address")
				}
				redistributionContractAddress = common.HexToAddress(o.RedistributionContractAddress)
			}
			redistributionContractABI, err := abi.JSON(strings.NewReader(chainCfg.RedistributionABI))
			if err != nil {
				return nil, fmt.Errorf("unable to parse redistribution ABI: %w", err)
			}

			redistributionContract := redistribution.New(swarmAddress, logger, transactionService, redistributionContractAddress, redistributionContractABI)
			agent = storageincentives.New(swarmAddress, chainBackend, logger, depthMonitor, redistributionContract, postageStampContractService, batchStore, storer, o.BlockTime, storageincentives.DefaultBlocksPerRound, storageincentives.DefaultBlocksPerPhase)
			b.storageIncetivesCloser = agent
		}
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
		BlockTime:        o.BlockTime,
		Tags:             tagService,
		Storer:           ns,
		Resolver:         multiResolver,
		Pss:              pssService,
		TraversalService: traversalService,
		Pinning:          pinningService,
		FeedFactory:      feedFactory,
		Post:             post,
		PostageContract:  postageStampContractService,
		Staking:          stakingContract,
		Steward:          steward,
		SyncStatus:       syncStatusFn,
		IndexDebugger:    storer,
	}

	if o.APIAddr != "" {
		if apiService == nil {
			apiService = api.New(*publicKey, pssPrivateKey.PublicKey, overlayEthAddress, logger, transactionService, batchStore, beeNodeMode, o.ChequebookEnable, o.SwapEnable, chainBackend, o.CORSAllowedOrigins)
			apiService.SetProbe(probe)
		}

		chunkC := apiService.Configure(signer, authenticator, tracer, api.Options{
			CORSAllowedOrigins: o.CORSAllowedOrigins,
			WsPingPeriod:       60 * time.Second,
			Restricted:         o.Restricted,
		}, extraOpts, chainID, erc20Service)

		pusherService.AddFeed(chunkC)

		apiService.MountAPI()

		if !o.Restricted {
			apiServer := &http.Server{
				IdleTimeout:       30 * time.Second,
				ReadHeaderTimeout: 3 * time.Second,
				Handler:           apiService,
				ErrorLog:          stdlog.New(b.errorLogWriter, "", 0),
			}

			apiListener, err := net.Listen("tcp", o.APIAddr)
			if err != nil {
				return nil, fmt.Errorf("api listener: %w", err)
			}

			go func() {
				logger.Info("starting api server", "address", apiListener.Addr())
				if err := apiServer.Serve(apiListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
					logger.Debug("api server failed to start", "error", err)
					logger.Error(nil, "api server failed to start")
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

		if agent != nil {
			debugService.MustRegisterMetrics(agent.Metrics()...)
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
		if ls, ok := eventListener.(metrics.Collector); ok {
			debugService.MustRegisterMetrics(ls.Metrics()...)
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
			WsPingPeriod:       60 * time.Second,
			Restricted:         o.Restricted,
		}, extraOpts, chainID, erc20Service)

		debugService.SetP2P(p2ps)
		debugService.SetSwarmAddress(&swarmAddress)
		debugService.MountDebug(false)
	}

	if err := kad.Start(ctx); err != nil {
		return nil, err
	}

	if err := p2ps.Ready(); err != nil {
		return nil, err
	}

	return b, nil
}

func (b *Bee) SyncingStopped() chan struct{} {
	return b.syncingStopped.C
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

	b.ctxCancel()
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
	tryClose(b.depthMonitorCloser, "depthmonitor service")
	tryClose(b.storageIncetivesCloser, "storage incentives agent")
	tryClose(b.stateStoreCloser, "statestore")
	tryClose(b.localstoreCloser, "localstore")
	tryClose(b.resolverCloser, "resolver service")

	return mErr
}

var ErrShutdownInProgress error = errors.New("shutdown in progress")

func isChainEnabled(o *Options, swapEndpoint string, logger log.Logger) bool {
	chainDisabled := swapEndpoint == ""
	lightMode := !o.FullNodeMode

	if lightMode && chainDisabled { // ultra light mode is LightNode mode with chain disabled
		logger.Info("starting with a disabled chain backend")
		return false
	}

	logger.Info("starting with an enabled chain backend")
	return true // all other modes operate require chain enabled
}
