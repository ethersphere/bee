//go:build js
// +build js

package node

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	stdlog "log"
	"math/big"
	"net/http"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	"github.com/ethersphere/bee/v2/pkg/accounting"
	"github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/feeds/factory"
	"github.com/ethersphere/bee/v2/pkg/gsoc"
	"github.com/ethersphere/bee/v2/pkg/hive"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/v2/pkg/pingpong"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/postage/batchstore"
	"github.com/ethersphere/bee/v2/pkg/pricer"
	"github.com/ethersphere/bee/v2/pkg/pricing"
	"github.com/ethersphere/bee/v2/pkg/pss"
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
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology/kademlia"
	"github.com/ethersphere/bee/v2/pkg/topology/lightnode"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/util/ioutil"
	"github.com/ethersphere/bee/v2/pkg/util/nbhdutil"
	"github.com/ethersphere/bee/v2/pkg/util/syncutil"
	ma "github.com/multiformats/go-multiaddr"

	wasmhttp "github.com/nlepage/go-wasm-http-server/v2"
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

	stateStore, _, err := InitStateStore(logger, o.DataDir, o.StatestoreCacheCapacity)
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

	var resetReserve bool
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
			reserveCapacity,
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
		return nil, fmt.Errorf("connected to wrong blockchain network; network chainID %d; configured chainID %d", chainID, o.ChainID)
	}

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
			logger.Info("starting debug & api server", "address")

			if _, err := wasmhttp.Serve(apiServer.Handler); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
		chequebookFactory, err = InitChequebookFactory(logger, chainBackend, chainID, transactionService, o.SwapFactoryAddress)
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

	detector.OnStabilized = func(t time.Time, totalCount int) {
		logger.Info("node warmup complete. system is considered stable and ready.", "stabilizationTime", t, "totalMonitoredEvents", totalCount)
	}

	detector.OnPeriodComplete = func(t time.Time, periodCount int, stDev float64) {
		logger.Debug("node warmup check: period complete.", "periodEndTime", t, "eventsInPeriod", periodCount, "rateStdDev", stDev)
	}

	// Bootstrap node with postage snapshot only if it is running on mainnet, is a fresh
	// install or explicitly asked by user to resync
	if networkID == mainnetNetworkID && o.UsePostageSnapshot && (!batchStoreExists || o.Resync) {
		start := time.Now()
		logger.Info("cold postage start detected. fetching postage stamp snapshot from swarm")
		_, err = bootstrapNode(
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

	p2ps, err := libp2p.New(ctx, signer, networkID, swarmAddress, addr, addressbook, stateStore, lightNodes, logger, tracer, libp2p.Options{
		PrivateKey:      libp2pPrivateKey,
		NATAddr:         o.NATAddr,
		EnableWS:        o.EnableWS,
		WelcomeMessage:  o.WelcomeMessage,
		FullNode:        o.FullNodeMode,
		Nonce:           nonce,
		ValidateOverlay: chainEnabled,
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

	fmt.Println("Supported protocols: ", p2ps.Protocols())

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

	nodeStatus := status.NewService(logger, p2ps, kad, beeNodeMode.String(), batchStore, localStore, nil)
	if err = p2ps.AddProtocol(nodeStatus.Protocol()); err != nil {
		return nil, fmt.Errorf("status service: %w", err)
	}

	saludService := salud.New(nodeStatus, kad, localStore, logger, detector, api.FullMode.String(), salud.DefaultMinPeersPerBin, salud.DefaultDurPercentile, salud.DefaultConnsPercentile)
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

	var (
		agent *storageincentives.Agent
	)

	multiResolver := multiresolver.NewMultiResolver(
		multiresolver.WithConnectionConfigs(o.ResolverConnectionCfgs),
		multiresolver.WithLogger(o.Logger),
		multiresolver.WithDefaultCIDResolver(),
	)
	b.resolverCloser = multiResolver

	feedFactory := factory.New(localStore.Download(true))
	steward := steward.New(localStore, retrieval, localStore.Cache())

	extraOpts := api.ExtraOptions{
		Pingpong:       pingPong,
		TopologyDriver: kad,
		LightNodes:     lightNodes,
		Accounting:     acc,
		Pseudosettle:   pseudosettleService,
		Swap:           swapService,
		Chequebook:     chequebookService,
		BlockTime:      o.BlockTime,
		Storer:         localStore,
		Resolver:       multiResolver,
		Pss:            pssService,
		Gsoc:           gsocService,
		FeedFactory:    feedFactory,
		Post:           post,
		AccessControl:  accesscontrol,
		Steward:        steward,
		SyncStatus:     syncStatusFn,
		NodeStatus:     nodeStatus,
		PinIntegrity:   localStore.PinIntegrity(),
	}

	if o.APIAddr != "" {
		// register metrics from components

		apiService.Configure(signer, tracer, api.Options{
			CORSAllowedOrigins: o.CORSAllowedOrigins,
			WsPingPeriod:       60 * time.Second,
		}, extraOpts, chainID, erc20Service)

		apiService.EnableFullAPI()

		apiService.SetRedistributionAgent(agent)

	}

	if err := kad.Start(ctx); err != nil {
		return nil, fmt.Errorf("start kademlia: %w", err)
	}

	if err := p2ps.Ready(); err != nil {
		return nil, fmt.Errorf("p2ps ready: %w", err)
	}

	return b, nil
}
