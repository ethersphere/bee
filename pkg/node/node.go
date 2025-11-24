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
	stdlog "log"
	"math/big"
	"net"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	"github.com/ethersphere/bee/v2/pkg/accounting"
	"github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/config"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/feeds/factory"
	"github.com/ethersphere/bee/v2/pkg/gsoc"
	"github.com/ethersphere/bee/v2/pkg/hive"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/v2/pkg/pingpong"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/postage/batchservice"
	"github.com/ethersphere/bee/v2/pkg/postage/batchstore"
	"github.com/ethersphere/bee/v2/pkg/postage/listener"
	"github.com/ethersphere/bee/v2/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/v2/pkg/pricer"
	"github.com/ethersphere/bee/v2/pkg/pricing"
	"github.com/ethersphere/bee/v2/pkg/pss"
	"github.com/ethersphere/bee/v2/pkg/puller"
	"github.com/ethersphere/bee/v2/pkg/pullsync"
	"github.com/ethersphere/bee/v2/pkg/pusher"
	"github.com/ethersphere/bee/v2/pkg/pushsync"
	"github.com/ethersphere/bee/v2/pkg/resolver/multiresolver"
	"github.com/ethersphere/bee/v2/pkg/retrieval"
	"github.com/ethersphere/bee/v2/pkg/salud"
	"github.com/ethersphere/bee/v2/pkg/settlement/pseudosettle"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/erc20"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/priceoracle"
	"github.com/ethersphere/bee/v2/pkg/stabilization"
	"github.com/ethersphere/bee/v2/pkg/status"
	"github.com/ethersphere/bee/v2/pkg/steward"
	"github.com/ethersphere/bee/v2/pkg/storageincentives"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/staking"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/topology/kademlia"
	"github.com/ethersphere/bee/v2/pkg/topology/lightnode"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/util/abiutil"
	"github.com/ethersphere/bee/v2/pkg/util/ioutil"
	"github.com/ethersphere/bee/v2/pkg/util/nbhdutil"
	"github.com/ethersphere/bee/v2/pkg/util/syncutil"
	"github.com/hashicorp/go-multierror"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/prometheus/client_golang/prometheus"
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
	resolverCloser           io.Closer
	errorLogWriter           io.Writer
	tracerCloser             io.Closer
	stateStoreCloser         io.Closer
	stamperStoreCloser       io.Closer
	localstoreCloser         io.Closer
	topologyCloser           io.Closer
	topologyHalter           topology.Halter
	pusherCloser             io.Closer
	pullerCloser             io.Closer
	accountingCloser         io.Closer
	pullSyncCloser           io.Closer
	pssCloser                io.Closer
	gsocCloser               io.Closer
	transactionMonitorCloser io.Closer
	transactionCloser        io.Closer
	listenerCloser           io.Closer
	postageServiceCloser     io.Closer
	priceOracleCloser        io.Closer
	hiveCloser               io.Closer
	saludCloser              io.Closer
	storageIncetivesCloser   io.Closer
	pushSyncCloser           io.Closer
	retrievalCloser          io.Closer
	shutdownInProgress       bool
	shutdownMutex            sync.Mutex
	syncingStopped           *syncutil.Signaler
	accesscontrolCloser      io.Closer
	ethClientCloser          func()
}

type Options struct {
	Addr                          string
	AllowPrivateCIDRs             bool
	APIAddr                       string
	AutoTLSEnabled                bool
	WSSAddr                       string
	AutoTLSStorageDir             string
	BlockchainRpcEndpoint         string
	BlockProfile                  bool
	BlockTime                     time.Duration
	BootnodeMode                  bool
	Bootnodes                     []string
	CacheCapacity                 uint64
	AutoTLSCAEndpoint             string
	ChainID                       int64
	ChequebookEnable              bool
	CORSAllowedOrigins            []string
	DataDir                       string
	DBBlockCacheCapacity          uint64
	DBDisableSeeksCompaction      bool
	DBOpenFilesLimit              uint64
	DBWriteBufferSize             uint64
	EnableStorageIncentives       bool
	EnableWS                      bool
	AutoTLSDomain                 string
	AutoTLSRegistrationEndpoint   string
	FullNodeMode                  bool
	Logger                        log.Logger
	MinimumGasTipCap              uint64
	MinimumStorageRadius          uint
	MutexProfile                  bool
	NATAddr                       string
	NATWSSAddr                    string
	NeighborhoodSuggester         string
	PaymentEarly                  int64
	PaymentThreshold              string
	PaymentTolerance              int64
	PostageContractAddress        string
	PostageContractStartBlock     uint64
	PriceOracleAddress            string
	RedistributionContractAddress string
	ReserveCapacityDoubling       int
	ResolverConnectionCfgs        []multiresolver.ConnectionConfig
	Resync                        bool
	RetrievalCaching              bool
	SkipPostageSnapshot           bool
	StakingContractAddress        string
	StatestoreCacheCapacity       uint64
	StaticNodes                   []swarm.Address
	SwapEnable                    bool
	SwapFactoryAddress            string
	SwapInitialDeposit            string
	TargetNeighborhood            string
	TracingEnabled                bool
	TracingEndpoint               string
	TracingServiceName            string
	TrxDebugMode                  bool
	UsePostageSnapshot            bool
	WarmupTime                    time.Duration
	WelcomeMessage                string
	WhitelistedWithdrawalAddress  []string
}

const (
	refreshRate                   = int64(4_500_000)          // accounting units refreshed per second
	lightFactor                   = 10                        // downscale payment thresholds and their change rate, and refresh rates by this for light nodes
	lightRefreshRate              = refreshRate / lightFactor // refresh rate used by / for light nodes
	basePrice                     = 10_000                    // minimal price for retrieval and pushsync requests of maximum proximity
	postageSyncingStallingTimeout = 10 * time.Minute          //
	postageSyncingBackoffTimeout  = 5 * time.Second           //
	minPaymentThreshold           = 2 * refreshRate           // minimal accepted payment threshold of full nodes
	maxPaymentThreshold           = 24 * refreshRate          // maximal accepted payment threshold of full nodes
	mainnetNetworkID              = uint64(1)                 //
	reserveWakeUpDuration         = 15 * time.Minute          // time to wait before waking up reserveWorker
	reserveMinEvictCount          = 1_000
	cacheMinEvictCount            = 10_000
	maxAllowedDoubling            = 1
)

func NewBee(
	ctx context.Context,
	addr string,
	publicKey *ecdsa.PublicKey,
	signer crypto.Signer,
	networkID uint64,
	logger log.Logger,
	libp2pPrivateKey,
	pssPrivateKey *ecdsa.PrivateKey,
	session accesscontrol.Session,
	o *Options,
) (b *Bee, err error) {
	// start time for node warmup duration measurement
	warmupStartTime := time.Now()
	var pullSyncStartTime time.Time

	nodeMetrics := newMetrics()

	tracer, tracerCloser, err := tracing.NewTracer(&tracing.Options{
		Enabled:     o.TracingEnabled,
		Endpoint:    o.TracingEndpoint,
		ServiceName: o.TracingServiceName,
	})
	if err != nil {
		return nil, fmt.Errorf("tracer: %w", err)
	}

	if err := validatePublicAddress(o.NATAddr); err != nil {
		return nil, fmt.Errorf("invalid NAT address %s: %w", o.NATAddr, err)
	}

	if err := validatePublicAddress(o.NATWSSAddr); err != nil {
		return nil, fmt.Errorf("invalid NAT WSS address %s: %w", o.NATWSSAddr, err)
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
		syncingStopped: syncutil.NewSignaler(),
	}

	defer func(b *Bee) {
		if err != nil {
			logger.Error(err, "got error, shutting down...")
			if err2 := b.Shutdown(); err2 != nil {
				logger.Error(err2, "got error while shutting down")
			}
		}
	}(b)

	if !o.FullNodeMode && o.ReserveCapacityDoubling != 0 {
		return nil, fmt.Errorf("reserve capacity doubling is only allowed for full nodes")
	}

	if o.ReserveCapacityDoubling < 0 || o.ReserveCapacityDoubling > maxAllowedDoubling {
		return nil, fmt.Errorf("config reserve capacity doubling has to be between default: 0 and maximum: %d", maxAllowedDoubling)
	}
	shallowReceiptTolerance := maxAllowedDoubling - o.ReserveCapacityDoubling

	reserveCapacity := (1 << o.ReserveCapacityDoubling) * storer.DefaultReserveCapacity

	stateStore, stateStoreMetrics, err := InitStateStore(logger, o.DataDir, o.StatestoreCacheCapacity)
	if err != nil {
		return nil, fmt.Errorf("init state store: %w", err)
	}

	pubKey, err := signer.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("signer public key: %w", err)
	}

	nonce, nonceExists, err := overlayNonceExists(stateStore)
	if err != nil {
		return nil, fmt.Errorf("check presence of nonce: %w", err)
	}

	swarmAddress, err := crypto.NewOverlayAddress(*pubKey, networkID, nonce)
	if err != nil {
		return nil, fmt.Errorf("compute overlay address: %w", err)
	}

	targetNeighborhood := o.TargetNeighborhood
	if targetNeighborhood == "" && !nonceExists && o.NeighborhoodSuggester != "" {
		logger.Info("fetching target neighborhood from suggester", "url", o.NeighborhoodSuggester)
		targetNeighborhood, err = nbhdutil.FetchNeighborhood(&http.Client{}, o.NeighborhoodSuggester)
		if err != nil {
			return nil, fmt.Errorf("neighborhood suggestion: %w", err)
		}
	}

	var changedOverlay, resetReserve bool
	if targetNeighborhood != "" {
		neighborhood, err := swarm.ParseBitStrAddress(targetNeighborhood)
		if err != nil {
			return nil, fmt.Errorf("invalid neighborhood. %s", targetNeighborhood)
		}

		if swarm.Proximity(swarmAddress.Bytes(), neighborhood.Bytes()) < uint8(len(targetNeighborhood)) {
			// mine the overlay
			logger.Info("mining a new overlay address to target the selected neighborhood", "target", targetNeighborhood)
			newSwarmAddress, newNonce, err := nbhdutil.MineOverlay(ctx, *pubKey, networkID, targetNeighborhood)
			if err != nil {
				return nil, fmt.Errorf("mine overlay address: %w", err)
			}

			if nonceExists {
				logger.Info("Override nonce and clean state for neighborhood", "old_none", hex.EncodeToString(nonce), "new_nonce", hex.EncodeToString(newNonce))
				logger.Warning("you have another 10 seconds to change your mind and kill this process with CTRL-C...")
				time.Sleep(10 * time.Second)

				err := ioutil.RemoveContent(filepath.Join(o.DataDir, ioutil.DataPathKademlia))
				if err != nil {
					return nil, fmt.Errorf("delete %s: %w", ioutil.DataPathKademlia, err)
				}

				if err := stateStore.ClearForHopping(); err != nil {
					return nil, fmt.Errorf("clearing stateStore %w", err)
				}
				resetReserve = true
			}

			swarmAddress = newSwarmAddress
			nonce = newNonce
			err = setOverlay(stateStore, swarmAddress, nonce)
			if err != nil {
				return nil, fmt.Errorf("statestore: save new overlay: %w", err)
			}
			changedOverlay = true
		}
	}

	b.stateStoreCloser = stateStore
	// Check if the batchstore exists. If not, we can assume it's missing
	// due to a migration or it's a fresh install.
	batchStoreExists, err := batchStoreExists(stateStore)
	if err != nil {
		return nil, fmt.Errorf("batchstore: exists: %w", err)
	}

	addressbook := addressbook.New(stateStore)

	logger.Info("using overlay address", "address", swarmAddress)

	// this will set overlay if it was not set before
	if err = checkOverlay(stateStore, swarmAddress); err != nil {
		return nil, fmt.Errorf("check overlay address: %w", err)
	}

	var (
		chequebookService chequebook.Service = new(noOpChequebookService)
		chequeStore       chequebook.ChequeStore
		cashoutService    chequebook.CashoutService
		erc20Service      erc20.Service
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
			reserveCapacity,
			logger,
		)
		if err != nil {
			return nil, fmt.Errorf("batchstore: %w", err)
		}
	}

	chainBackend, overlayEthAddress, chainID, transactionMonitor, transactionService, err := InitChain(
		ctx,
		logger,
		stateStore,
		o.BlockchainRpcEndpoint,
		o.ChainID,
		signer,
		o.BlockTime,
		chainEnabled,
		o.MinimumGasTipCap,
	)
	if err != nil {
		return nil, fmt.Errorf("init chain: %w", err)
	}

	logger.Info("using chain with network", "chain_id", chainID, "network_id", networkID)

	b.ethClientCloser = chainBackend.Close
	b.transactionCloser = tracerCloser
	b.transactionMonitorCloser = transactionMonitor

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

	stamperStore, err := InitStamperStore(logger, o.DataDir, stateStore)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize stamper store: %w", err)
	}
	b.stamperStoreCloser = stamperStore

	var apiService *api.Service

	if o.APIAddr != "" {
		if o.MutexProfile {
			_ = runtime.SetMutexProfileFraction(1)
		}
		if o.BlockProfile {
			runtime.SetBlockProfileRate(1)
		}

		apiListener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", o.APIAddr)
		if err != nil {
			return nil, fmt.Errorf("api listener: %w", err)
		}

		apiService = api.New(
			*publicKey,
			pssPrivateKey.PublicKey,
			overlayEthAddress,
			o.WhitelistedWithdrawalAddress,
			logger,
			transactionService,
			batchStore,
			beeNodeMode,
			o.ChequebookEnable,
			o.SwapEnable,
			chainBackend,
			o.CORSAllowedOrigins,
			stamperStore,
		)

		apiService.Mount()
		apiService.SetProbe(probe)
		apiService.SetIsWarmingUp(true)
		apiService.SetSwarmAddress(&swarmAddress)

		apiServer := &http.Server{
			IdleTimeout:       30 * time.Second,
			ReadHeaderTimeout: 3 * time.Second,
			Handler:           apiService,
			ErrorLog:          stdlog.New(b.errorLogWriter, "", 0),
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
		logger.Info("waiting to sync with the blockchain backend")

		err := transaction.WaitSynced(ctx, logger, chainBackend, maxDelay)
		if err != nil {
			return nil, fmt.Errorf("waiting backend sync: %w", err)
		}
	}

	if o.SwapEnable {
		chequebookFactory, err := InitChequebookFactory(logger, chainBackend, chainID, transactionService, o.SwapFactoryAddress)
		if err != nil {
			return nil, fmt.Errorf("init chequebook factory: %w", err)
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
				erc20Service,
			)
			if err != nil {
				return nil, fmt.Errorf("init chequebook service: %w", err)
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

	detector, err := stabilization.NewDetector(stabilization.Config{
		PeriodDuration:             2 * time.Second,
		NumPeriodsForStabilization: 5,
		StabilizationFactor:        3,
		MinimumPeriods:             2,
		WarmupTime:                 warmupTime,
	})
	if err != nil {
		return nil, fmt.Errorf("rate stabilizer configuration failed: %w", err)
	}
	defer detector.Close()

	detector.OnMonitoringStart = func(t time.Time) {
		logger.Info("node warmup check initiated. monitoring activity rate to determine readiness.", "startTime", t)
	}

	warmupMeasurement := func(t time.Time, totalCount int) {
		warmupDuration := t.Sub(warmupStartTime).Seconds()
		logger.Info("node warmup complete. system is considered stable and ready.",
			"stabilizationTime", t,
			"totalMonitoredEvents", totalCount,
			"warmupDurationSeconds", warmupDuration)

		nodeMetrics.WarmupDuration.Observe(warmupDuration)
		pullSyncStartTime = t
	}
	detector.OnStabilized = warmupMeasurement

	detector.OnPeriodComplete = func(t time.Time, periodCount int, stDev float64) {
		logger.Debug("node warmup check: period complete.", "periodEndTime", t, "eventsInPeriod", periodCount, "rateStdDev", stDev)
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
			addressbook,
			bootnodes,
			lightNodes,
			stateStore,
			signer,
			networkID,
			log.Noop,
			libp2pPrivateKey,
			detector,
			o,
		)
		logger.Info("bootstrapper created", "elapsed", time.Since(start))
		if err != nil {
			logger.Error(err, "bootstrapper failed to fetch batch state")
		}
	}

	var registry *prometheus.Registry

	if apiService != nil {
		registry = apiService.MetricsRegistry()
	}

	p2ps, err := libp2p.New(ctx, signer, networkID, swarmAddress, addr, addressbook, stateStore, lightNodes, logger, tracer, libp2p.Options{
		PrivateKey:                  libp2pPrivateKey,
		NATAddr:                     o.NATAddr,
		NATWSSAddr:                  o.NATWSSAddr,
		EnableWS:                    o.EnableWS,
		AutoTLSEnabled:              o.AutoTLSEnabled,
		WSSAddr:                     o.WSSAddr,
		AutoTLSStorageDir:           o.AutoTLSStorageDir,
		AutoTLSDomain:               o.AutoTLSDomain,
		AutoTLSRegistrationEndpoint: o.AutoTLSRegistrationEndpoint,
		AutoTLSCAEndpoint:           o.AutoTLSCAEndpoint,
		WelcomeMessage:              o.WelcomeMessage,
		FullNode:                    o.FullNodeMode,
		Nonce:                       nonce,
		ValidateOverlay:             chainEnabled,
		Registry:                    registry,
	})
	if err != nil {
		return nil, fmt.Errorf("p2p service: %w", err)
	}

	apiService.SetP2P(p2ps)

	b.p2pService = p2ps
	b.p2pHalter = p2ps

	post, err := postage.NewService(logger, stamperStore, batchStore, chainID)
	if err != nil {
		return nil, fmt.Errorf("postage service: %w", err)
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

	postageStampContractABI := abiutil.MustParseABI(chainCfg.PostageStampABI)

	bzzTokenAddress, err := postagecontract.LookupERC20Address(ctx, transactionService, postageStampContractAddress, postageStampContractABI, chainEnabled)
	if err != nil {
		return nil, fmt.Errorf("lookup erc20 postage address: %w", err)
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
		o.TrxDebugMode,
	)

	eventListener = listener.New(b.syncingStopped, logger, chainBackend, postageStampContractAddress, postageStampContractABI, o.BlockTime, postageSyncingStallingTimeout, postageSyncingBackoffTimeout)
	b.listenerCloser = eventListener

	batchSvc, err = batchservice.New(stateStore, batchStore, logger, eventListener, overlayEthAddress.Bytes(), post, sha3.New256, o.Resync)
	if err != nil {
		return nil, fmt.Errorf("init batch service: %w", err)
	}

	// Construct protocols.
	pingPong := pingpong.New(p2ps, logger, tracer)

	if err = p2ps.AddProtocol(pingPong.Protocol()); err != nil {
		return nil, fmt.Errorf("pingpong service: %w", err)
	}

	hive := hive.New(p2ps, addressbook, networkID, o.BootnodeMode, o.AllowPrivateCIDRs, logger)

	if err = p2ps.AddProtocol(hive.Protocol()); err != nil {
		return nil, fmt.Errorf("hive service: %w", err)
	}
	b.hiveCloser = hive

	var swapService *swap.Service

	kad, err := kademlia.New(swarmAddress, addressbook, hive, p2ps, detector, logger,
		kademlia.Options{Bootnodes: bootnodes, BootnodeMode: o.BootnodeMode, StaticNodes: o.StaticNodes, DataDir: o.DataDir})
	if err != nil {
		return nil, fmt.Errorf("unable to create kademlia: %w", err)
	}
	b.topologyCloser = kad
	b.topologyHalter = kad
	hive.SetAddPeersHandler(kad.AddPeers)
	p2ps.SetPickyNotifier(kad)

	var path string

	if o.DataDir != "" {
		logger.Info("using datadir", "path", o.DataDir)
		path = filepath.Join(o.DataDir, ioutil.DataPathLocalstore)
	}

	lo := &storer.Options{
		Address:                   swarmAddress,
		CacheCapacity:             o.CacheCapacity,
		LdbOpenFilesLimit:         o.DBOpenFilesLimit,
		LdbBlockCacheCapacity:     o.DBBlockCacheCapacity,
		LdbWriteBufferSize:        o.DBWriteBufferSize,
		LdbDisableSeeksCompaction: o.DBDisableSeeksCompaction,
		Batchstore:                batchStore,
		StateStore:                stateStore,
		RadiusSetter:              kad,
		StartupStabilizer:         detector,
		Logger:                    logger,
		Tracer:                    tracer,
		CacheMinEvictCount:        cacheMinEvictCount,
		MinimumStorageRadius:      o.MinimumStorageRadius,
	}

	if o.FullNodeMode && !o.BootnodeMode {
		// configure reserve only for full node
		lo.ReserveCapacity = reserveCapacity
		lo.ReserveWakeUpDuration = reserveWakeUpDuration
		lo.ReserveMinEvictCount = reserveMinEvictCount
		lo.RadiusSetter = kad
		lo.ReserveCapacityDoubling = o.ReserveCapacityDoubling
	}

	localStore, err := storer.New(ctx, path, lo)
	if err != nil {
		return nil, fmt.Errorf("localstore: %w", err)
	}
	b.localstoreCloser = localStore
	evictFn = func(id []byte) error { return localStore.EvictBatch(context.Background(), id) }

	if resetReserve {
		logger.Warning("resetting the reserve")
		err := localStore.ResetReserve(ctx)
		if err != nil {
			return nil, fmt.Errorf("reset reserve: %w", err)
		}
	}

	actLogic := accesscontrol.NewLogic(session)
	accesscontrol := accesscontrol.NewController(actLogic)
	b.accesscontrolCloser = accesscontrol

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

	if !o.SkipPostageSnapshot && !batchStoreExists && (networkID == mainnetNetworkID) && beeNodeMode != api.UltraLightMode {
		chainBackend := NewSnapshotLogFilterer(logger, archiveSnapshotGetter{})

		snapshotEventListener := listener.New(b.syncingStopped, logger, chainBackend, postageStampContractAddress, postageStampContractABI, o.BlockTime, postageSyncingStallingTimeout, postageSyncingBackoffTimeout)

		snapshotBatchSvc, err := batchservice.New(stateStore, batchStore, logger, snapshotEventListener, overlayEthAddress.Bytes(), post, sha3.New256, o.Resync)
		if err != nil {
			logger.Error(err, "failed to initialize batch service from snapshot, continuing outside snapshot block...")
		} else {
			err = snapshotBatchSvc.Start(ctx, postageSyncStart, initBatchState)
			syncStatus.Store(true)
			if err != nil {
				syncErr.Store(err)
				logger.Error(err, "failed to start batch service from snapshot, continuing outside snapshot block...")
			} else {
				postageSyncStart = chainBackend.maxBlockHeight
			}
		}
		if errClose := snapshotEventListener.Close(); errClose != nil {
			logger.Error(errClose, "failed to close event listener (snapshot) failure")
		}

	}

	if batchSvc != nil && chainEnabled {
		logger.Info("waiting to sync postage contract data, this may take a while... more info available in Debug loglevel")

		paused, err := postageStampContractService.Paused(ctx)
		if err != nil {
			logger.Error(err, "Error checking postage contract is paused")
		}

		if paused {
			return nil, errors.New("postage contract is paused")
		}

		if o.FullNodeMode {
			err = batchSvc.Start(ctx, postageSyncStart, initBatchState)
			syncStatus.Store(true)
			if err != nil {
				syncErr.Store(err)
				return nil, fmt.Errorf("unable to start batch service: %w", err)
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
			return nil, fmt.Errorf("init swap service: %w", err)
		}
		b.priceOracleCloser = priceOracle

		if o.ChequebookEnable {
			acc.SetPayFunc(swapService.Pay)
		}
	}

	pricing.SetPaymentThresholdObserver(acc)

	pssService := pss.New(pssPrivateKey, logger)
	gsocService := gsoc.New(logger)
	b.pssCloser = pssService
	b.gsocCloser = gsocService

	validStamp := postage.ValidStamp(batchStore)

	// metrics exposed on the status protocol
	statusMetricsRegistry := prometheus.NewRegistry()
	if localStore != nil {
		statusMetricsRegistry.MustRegister(localStore.StatusMetrics()...)
	}
	if p2ps != nil {
		statusMetricsRegistry.MustRegister(p2ps.StatusMetrics()...)
	}

	nodeStatus := status.NewService(logger, p2ps, kad, beeNodeMode.String(), batchStore, localStore, statusMetricsRegistry)
	if err = p2ps.AddProtocol(nodeStatus.Protocol()); err != nil {
		return nil, fmt.Errorf("status service: %w", err)
	}

	saludService := salud.New(nodeStatus, kad, localStore, logger, detector, api.FullMode.String(), salud.DefaultDurPercentile, salud.DefaultConnsPercentile)
	b.saludCloser = saludService

	rC, unsub := saludService.SubscribeNetworkStorageRadius()
	initialRadiusC := make(chan struct{})
	var networkR atomic.Uint32
	networkR.Store(uint32(swarm.MaxBins))

	go func() {
		for {
			select {
			case r := <-rC:
				prev := networkR.Load()
				networkR.Store(uint32(r))
				if prev == uint32(swarm.MaxBins) {
					close(initialRadiusC)
				}
				if !o.FullNodeMode { // light and ultra-light nodes do not have a reserve worker to set the radius.
					kad.SetStorageRadius(r)
				}
			case <-ctx.Done():
				unsub()
				return
			}
		}
	}()

	waitNetworkRFunc := func() (uint8, error) {
		if networkR.Load() == uint32(swarm.MaxBins) {
			select {
			case <-initialRadiusC:
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}

		local, network := localStore.StorageRadius(), uint8(networkR.Load())
		if local <= uint8(o.MinimumStorageRadius) {
			return max(network, uint8(o.MinimumStorageRadius)), nil
		} else {
			return local, nil
		}
	}

	pushSyncProtocol := pushsync.New(swarmAddress, networkID, nonce, p2ps, localStore, waitNetworkRFunc, kad, o.FullNodeMode && !o.BootnodeMode, pssService.TryUnwrap, gsocService.Handle, validStamp, logger, acc, pricer, signer, tracer, detector, uint8(shallowReceiptTolerance))
	b.pushSyncCloser = pushSyncProtocol

	// set the pushSyncer in the PSS
	pssService.SetPushSyncer(pushSyncProtocol)

	retrieval := retrieval.New(swarmAddress, waitNetworkRFunc, localStore, p2ps, kad, logger, acc, pricer, tracer, o.RetrievalCaching)
	localStore.SetRetrievalService(retrieval)

	statusMetricsRegistry.MustRegister(retrieval.StatusMetrics()...)

	pusherService := pusher.New(networkID, localStore, pushSyncProtocol, batchStore, logger, detector, pusher.DefaultRetryCount)
	b.pusherCloser = pusherService

	pusherService.AddFeed(localStore.PusherFeed())

	pullSyncProtocol := pullsync.New(p2ps, localStore, pssService.TryUnwrap, gsocService.Handle, validStamp, logger, pullsync.DefaultMaxPage)
	b.pullSyncCloser = pullSyncProtocol

	retrieveProtocolSpec := retrieval.Protocol()
	pushSyncProtocolSpec := pushSyncProtocol.Protocol()
	pullSyncProtocolSpec := pullSyncProtocol.Protocol()

	if o.FullNodeMode && !o.BootnodeMode {
		logger.Info("starting in full mode")
	} else {
		if chainEnabled {
			logger.Info("starting in light mode")
		} else {
			logger.Info("starting in ultra-light mode")
		}
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

	go func() {
		sub, unsubscribe := detector.Subscribe()
		defer unsubscribe()
		<-sub
		logger.Info("node warmup stabilization complete, updating API status")
		apiService.SetIsWarmingUp(false)
	}()

	stakingContractAddress := chainCfg.StakingAddress
	if o.StakingContractAddress != "" {
		if !common.IsHexAddress(o.StakingContractAddress) {
			return nil, errors.New("malformed staking contract address")
		}
		stakingContractAddress = common.HexToAddress(o.StakingContractAddress)
	}

	stakingContract := staking.New(overlayEthAddress, stakingContractAddress, abiutil.MustParseABI(chainCfg.StakingABI), bzzTokenAddress, transactionService, common.BytesToHash(nonce), o.TrxDebugMode, uint8(o.ReserveCapacityDoubling))

	if chainEnabled {

		stake, err := stakingContract.GetPotentialStake(ctx)
		if err != nil {
			return nil, fmt.Errorf("get potential stake: %w", err)
		}

		if stake.Cmp(big.NewInt(0)) > 0 {

			if changedOverlay {
				logger.Debug("changing overlay address in staking contract")
				tx, err := stakingContract.ChangeStakeOverlay(ctx, common.BytesToHash(nonce))
				if err != nil {
					return nil, fmt.Errorf("cannot change staking overlay address: %v", err.Error())
				}
				logger.Info("overlay address changed in staking contract", "transaction", tx)
			}

			// make sure that the staking contract has the up to date height
			tx, updated, err := stakingContract.UpdateHeight(ctx)
			if err != nil {
				return nil, fmt.Errorf("update height in staking contract: %w", err)
			}
			if updated {
				logger.Info("updated new reserve capacity doubling height in the staking contract", "transaction", tx, "new_height", o.ReserveCapacityDoubling)
			}

			// Check if the staked amount is sufficient to cover the additional neighborhoods.
			// The staked amount must be at least 2^h * MinimumStake.
			if o.ReserveCapacityDoubling > 0 && stake.Cmp(big.NewInt(0).Mul(big.NewInt(1<<o.ReserveCapacityDoubling), staking.MinimumStakeAmount)) < 0 {
				logger.Warning("staked amount does not sufficiently cover the additional reserve capacity. Stake should be at least 2^h * 10 BZZ, where h is the number extra doublings.")
			}
		}
	}

	var (
		pullerService *puller.Puller
		agent         *storageincentives.Agent
	)

	if o.FullNodeMode && !o.BootnodeMode {
		pullerService = puller.New(swarmAddress, stateStore, kad, localStore, pullSyncProtocol, p2ps, logger, puller.Options{})
		b.pullerCloser = pullerService

		localStore.StartReserveWorker(ctx, pullerService, waitNetworkRFunc)
		nodeStatus.SetSync(pullerService)

		// measure full sync duration
		detector.OnStabilized = func(t time.Time, totalCount int) {
			warmupMeasurement(t, totalCount)

			reserveTreshold := reserveCapacity >> 1
			isFullySynced := func() bool {
				return pullerService.SyncRate() == 0 && saludService.IsHealthy() && localStore.ReserveSize() >= reserveTreshold
			}

			syncCheckTicker := time.NewTicker(2 * time.Second)
			go func() {
				defer syncCheckTicker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-syncCheckTicker.C:
						synced := isFullySynced()
						logger.Debug("sync status check", "synced", synced, "reserveSize", localStore.ReserveSize(), "syncRate", pullerService.SyncRate())
						if synced {
							fullSyncTime := pullSyncStartTime.Sub(t)
							logger.Info("full sync done", "duration", fullSyncTime)
							nodeMetrics.FullSyncDuration.Observe(fullSyncTime.Minutes())
							syncCheckTicker.Stop()
							return
						}
					}
				}
			}()
		}

		if o.EnableStorageIncentives {

			redistributionContractAddress := chainCfg.RedistributionAddress
			if o.RedistributionContractAddress != "" {
				if !common.IsHexAddress(o.RedistributionContractAddress) {
					return nil, errors.New("malformed redistribution contract address")
				}
				redistributionContractAddress = common.HexToAddress(o.RedistributionContractAddress)
			}

			redistributionContract := redistribution.New(swarmAddress, overlayEthAddress, logger, transactionService, redistributionContractAddress, abiutil.MustParseABI(chainCfg.RedistributionABI), o.TrxDebugMode)

			isFullySynced := func() bool {
				reserveTreshold := reserveCapacity * 5 / 10
				logger.Debug("Sync status check evaluated", "stabilized", detector.IsStabilized())
				return localStore.ReserveSize() >= reserveTreshold && pullerService.SyncRate() == 0 && detector.IsStabilized()
			}

			agent, err = storageincentives.New(
				swarmAddress,
				overlayEthAddress,
				chainBackend,
				redistributionContract,
				postageStampContractService,
				stakingContract,
				localStore,
				isFullySynced,
				o.BlockTime,
				storageincentives.DefaultBlocksPerRound,
				storageincentives.DefaultBlocksPerPhase,
				stateStore,
				batchStore,
				erc20Service,
				transactionService,
				saludService,
				logger,
			)
			if err != nil {
				return nil, fmt.Errorf("storage incentives agent: %w", err)
			}
			b.storageIncetivesCloser = agent
		}

	}
	multiResolver := multiresolver.NewMultiResolver(
		multiresolver.WithConnectionConfigs(o.ResolverConnectionCfgs),
		multiresolver.WithLogger(o.Logger),
		multiresolver.WithDefaultCIDResolver(),
	)
	b.resolverCloser = multiResolver

	feedFactory := factory.New(localStore.Download(true))
	steward := steward.New(localStore, retrieval, localStore.Cache())

	extraOpts := api.ExtraOptions{
		Pingpong:        pingPong,
		TopologyDriver:  kad,
		LightNodes:      lightNodes,
		Accounting:      acc,
		Pseudosettle:    pseudosettleService,
		Swap:            swapService,
		Chequebook:      chequebookService,
		BlockTime:       o.BlockTime,
		Storer:          localStore,
		Resolver:        multiResolver,
		Pss:             pssService,
		Gsoc:            gsocService,
		FeedFactory:     feedFactory,
		Post:            post,
		AccessControl:   accesscontrol,
		PostageContract: postageStampContractService,
		Staking:         stakingContract,
		Steward:         steward,
		SyncStatus:      syncStatusFn,
		NodeStatus:      nodeStatus,
		PinIntegrity:    localStore.PinIntegrity(),
	}

	if o.APIAddr != "" {
		// register metrics from components
		apiService.MustRegisterMetrics(p2ps.Metrics()...)
		apiService.MustRegisterMetrics(pingPong.Metrics()...)
		apiService.MustRegisterMetrics(acc.Metrics()...)
		apiService.MustRegisterMetrics(localStore.Metrics()...)
		apiService.MustRegisterMetrics(kad.Metrics()...)
		apiService.MustRegisterMetrics(saludService.Metrics()...)
		apiService.MustRegisterMetrics(stateStoreMetrics.Metrics()...)
		apiService.MustRegisterMetrics(getMetrics(nodeMetrics)...)

		if pullerService != nil {
			apiService.MustRegisterMetrics(pullerService.Metrics()...)
		}

		if agent != nil {
			apiService.MustRegisterMetrics(agent.Metrics()...)
		}

		apiService.MustRegisterMetrics(pushSyncProtocol.Metrics()...)
		apiService.MustRegisterMetrics(pusherService.Metrics()...)
		apiService.MustRegisterMetrics(pullSyncProtocol.Metrics()...)
		apiService.MustRegisterMetrics(retrieval.Metrics()...)
		apiService.MustRegisterMetrics(lightNodes.Metrics()...)
		apiService.MustRegisterMetrics(hive.Metrics()...)

		if bs, ok := batchStore.(metrics.Collector); ok {
			apiService.MustRegisterMetrics(bs.Metrics()...)
		}
		if ls, ok := eventListener.(metrics.Collector); ok {
			apiService.MustRegisterMetrics(ls.Metrics()...)
		}
		if pssServiceMetrics, ok := pssService.(metrics.Collector); ok {
			apiService.MustRegisterMetrics(pssServiceMetrics.Metrics()...)
		}
		if swapBackendMetrics, ok := chainBackend.(metrics.Collector); ok {
			apiService.MustRegisterMetrics(swapBackendMetrics.Metrics()...)
		}

		if l, ok := logger.(metrics.Collector); ok {
			apiService.MustRegisterMetrics(l.Metrics()...)
		}
		apiService.MustRegisterMetrics(pseudosettleService.Metrics()...)
		if swapService != nil {
			apiService.MustRegisterMetrics(swapService.Metrics()...)
		}

		apiService.Configure(signer, tracer, api.Options{
			CORSAllowedOrigins: o.CORSAllowedOrigins,
			WsPingPeriod:       60 * time.Second,
		}, extraOpts, chainID, erc20Service)

		apiService.EnableFullAPI()

		apiService.SetRedistributionAgent(agent)

		// api metrics are constructed on api.Service.Configure
		statusMetricsRegistry.MustRegister(apiService.StatusMetrics()...)
	}

	if err := kad.Start(ctx); err != nil {
		return nil, fmt.Errorf("start kademlia: %w", err)
	}

	if err := p2ps.Ready(); err != nil {
		return nil, fmt.Errorf("p2ps ready: %w", err)
	}

	return b, nil
}

func (b *Bee) SyncingStopped() chan struct{} {
	return b.syncingStopped.C
}

// namedCloser is a helper struct to associate a closer with its name.
type namedCloser struct {
	closer io.Closer
	name   string
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
	if err := eg.Wait(); err != nil {
		mErr = multierror.Append(mErr, err)
	}

	var wg sync.WaitGroup

	closers := []namedCloser{
		{b.pssCloser, "pss"},
		{b.gsocCloser, "gsoc"},
		{b.pusherCloser, "pusher"},
		{b.pullerCloser, "puller"},
		{b.accountingCloser, "accounting"},
		{b.pullSyncCloser, "pull sync"},
		{b.hiveCloser, "hive"},
		{b.saludCloser, "salud"},
	}

	wg.Add(len(closers))
	for _, nc := range closers {
		go func(c io.Closer, name string) {
			defer wg.Done()
			tryClose(c, name)
		}(nc.closer, nc.name)
	}

	b.ctxCancel()
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

	if b.ethClientCloser != nil {
		b.ethClientCloser()
	}

	tryClose(b.accesscontrolCloser, "accesscontrol")
	tryClose(b.tracerCloser, "tracer")
	tryClose(b.topologyCloser, "topology driver")
	tryClose(b.storageIncetivesCloser, "storage incentives agent")
	tryClose(b.stateStoreCloser, "statestore")
	tryClose(b.stamperStoreCloser, "stamperstore")
	tryClose(b.localstoreCloser, "localstore")
	tryClose(b.resolverCloser, "resolver service")

	return mErr
}

var ErrShutdownInProgress = errors.New("shutdown in progress")

func isChainEnabled(o *Options, swapEndpoint string, logger log.Logger) bool {
	chainDisabled := swapEndpoint == ""
	lightMode := !o.FullNodeMode

	if lightMode && chainDisabled {
		logger.Info("chain backend disabled - starting in ultra-light mode",
			"full_node_mode", o.FullNodeMode,
			"blockchain-rpc-endpoint", swapEndpoint)
		return false
	}

	logger.Info("chain backend enabled - blockchain functionality available",
		"full_node_mode", o.FullNodeMode,
		"blockchain-rpc-endpoint", swapEndpoint)
	return true // all other modes operate require chain enabled
}

func validatePublicAddress(addr string) error {
	if addr == "" {
		return nil
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%w", err)
	}
	if host == "" {
		return errors.New("host is empty")
	}
	if port == "" {
		return errors.New("port is empty")
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return fmt.Errorf("port is not a valid number: %w", err)
	}
	if host == "localhost" {
		return errors.New("localhost is not a valid address")
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return errors.New("not a valid IP address")
	}
	if ip.IsLoopback() {
		return errors.New("loopback address is not a valid address")
	}
	if ip.IsPrivate() {
		return errors.New("private address is not a valid address")
	}

	return nil
}
