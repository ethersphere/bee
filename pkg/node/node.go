// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/content"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/hive"
	"github.com/ethersphere/bee/pkg/kademlia"
	"github.com/ethersphere/bee/pkg/keystore"
	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/metrics"
	"github.com/ethersphere/bee/pkg/netstore"
	"github.com/ethersphere/bee/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/pkg/pingpong"
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
	settlement "github.com/ethersphere/bee/pkg/settlement"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle"
	"github.com/ethersphere/bee/pkg/settlement/swap"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/settlement/swap/swapprotocol"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/statestore/leveldb"
	mockinmem "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/ethersphere/bee/pkg/traversal"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

type Bee struct {
	p2pService            io.Closer
	p2pCancel             context.CancelFunc
	apiCloser             io.Closer
	apiServer             *http.Server
	debugAPIServer        *http.Server
	resolverCloser        io.Closer
	errorLogWriter        *io.PipeWriter
	tracerCloser          io.Closer
	tagsCloser            io.Closer
	stateStoreCloser      io.Closer
	localstoreCloser      io.Closer
	topologyCloser        io.Closer
	pusherCloser          io.Closer
	pullerCloser          io.Closer
	pullSyncCloser        io.Closer
	pssCloser             io.Closer
	recoveryHandleCleanup func()
}

type Options struct {
	DataDir                string
	DBCapacity             uint64
	Password               string
	APIAddr                string
	DebugAPIAddr           string
	Addr                   string
	NATAddr                string
	EnableWS               bool
	EnableQUIC             bool
	WelcomeMessage         string
	Bootnodes              []string
	CORSAllowedOrigins     []string
	Logger                 logging.Logger
	Standalone             bool
	TracingEnabled         bool
	TracingEndpoint        string
	TracingServiceName     string
	GlobalPinningEnabled   bool
	PaymentThreshold       uint64
	PaymentTolerance       uint64
	PaymentEarly           uint64
	ResolverConnectionCfgs []multiresolver.ConnectionConfig
	GatewayMode            bool
	SwapEndpoint           string
	SwapFactoryAddress     string
	SwapInitialDeposit     uint64
	SwapEnable             bool
}

func NewBee(addr string, swarmAddress swarm.Address, publicKey ecdsa.PublicKey, keystore keystore.Service, signer crypto.Signer, networkID uint64, logger logging.Logger, o Options) (*Bee, error) {
	tracer, tracerCloser, err := tracing.NewTracer(&tracing.Options{
		Enabled:     o.TracingEnabled,
		Endpoint:    o.TracingEndpoint,
		ServiceName: o.TracingServiceName,
	})
	if err != nil {
		return nil, fmt.Errorf("tracer: %w", err)
	}

	p2pCtx, p2pCancel := context.WithCancel(context.Background())

	b := &Bee{
		p2pCancel:      p2pCancel,
		errorLogWriter: logger.WriterLevel(logrus.ErrorLevel),
		tracerCloser:   tracerCloser,
	}

	// Construct P2P service.
	libp2pPrivateKey, created, err := keystore.Key("libp2p", o.Password)
	if err != nil {
		return nil, fmt.Errorf("libp2p key: %w", err)
	}
	if created {
		logger.Debugf("new libp2p key created")
	} else {
		logger.Debugf("using existing libp2p key")
	}

	var stateStore storage.StateStorer
	if o.DataDir == "" {
		stateStore = mockinmem.NewStateStore()
		logger.Warning("using in-mem state store. no node state will be persisted")
	} else {
		stateStore, err = leveldb.NewStateStore(filepath.Join(o.DataDir, "statestore"))
		if err != nil {
			return nil, fmt.Errorf("statestore: %w", err)
		}
	}
	b.stateStoreCloser = stateStore
	addressbook := addressbook.New(stateStore)

	var chequebookService chequebook.Service
	var chequeStore chequebook.ChequeStore
	var cashoutService chequebook.CashoutService
	var overlayEthAddress common.Address
	if o.SwapEnable {
		swapBackend, err := ethclient.Dial(o.SwapEndpoint)
		if err != nil {
			return nil, err
		}
		transactionService, err := transaction.NewService(logger, swapBackend, signer)
		if err != nil {
			return nil, err
		}
		overlayEthAddress, err = signer.EthereumAddress()
		if err != nil {
			return nil, err
		}

		// print ethereum address so users know which address we need to fund
		logger.Infof("using ethereum address %x", overlayEthAddress)

		chainID, err := swapBackend.ChainID(p2pCtx)
		if err != nil {
			logger.Infof("could not connect to backend at %v. In a swap-enabled network a working blockchain node (for goerli network in production) is required. Check your node or specify another node using --swap-endpoint.", o.SwapEndpoint)
			return nil, fmt.Errorf("could not get chain id from ethereum backend: %w", err)
		}

		var factoryAddress common.Address
		if o.SwapFactoryAddress == "" {
			var found bool
			factoryAddress, found = chequebook.DiscoverFactoryAddress(chainID.Int64())
			if !found {
				return nil, errors.New("no known factory address for this network")
			}
			logger.Infof("using default factory address for chain id %d: %x", chainID, factoryAddress)
		} else if !common.IsHexAddress(o.SwapFactoryAddress) {
			return nil, errors.New("malformed factory address")
		} else {
			factoryAddress = common.HexToAddress(o.SwapFactoryAddress)
			logger.Infof("using custom factory address: %x", factoryAddress)
		}

		chequebookFactory, err := chequebook.NewFactory(swapBackend, transactionService, factoryAddress, chequebook.NewSimpleSwapFactoryBindingFunc)
		if err != nil {
			return nil, err
		}

		chequeSigner := chequebook.NewChequeSigner(signer, chainID.Int64())

		// initialize chequebook logic
		chequebookService, err = chequebook.Init(p2pCtx,
			chequebookFactory,
			stateStore,
			logger,
			o.SwapInitialDeposit,
			transactionService,
			swapBackend,
			chainID.Int64(),
			overlayEthAddress,
			chequeSigner,
			chequebook.NewSimpleSwapBindings,
			chequebook.NewERC20Bindings)
		if err != nil {
			return nil, err
		}

		chequeStore = chequebook.NewChequeStore(stateStore, swapBackend, chequebookFactory, chainID.Int64(), overlayEthAddress, chequebook.NewSimpleSwapBindings, chequebook.RecoverCheque)

		cashoutService, err = chequebook.NewCashoutService(stateStore, chequebook.NewSimpleSwapBindings, swapBackend, transactionService, chequeStore)
		if err != nil {
			return nil, err
		}
	}

	p2ps, err := libp2p.New(p2pCtx, signer, networkID, swarmAddress, addr, addressbook, stateStore, logger, tracer, libp2p.Options{
		PrivateKey:     libp2pPrivateKey,
		NATAddr:        o.NATAddr,
		EnableWS:       o.EnableWS,
		EnableQUIC:     o.EnableQUIC,
		Standalone:     o.Standalone,
		WelcomeMessage: o.WelcomeMessage,
	})
	if err != nil {
		return nil, fmt.Errorf("p2p service: %w", err)
	}
	b.p2pService = p2ps

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

	var settlement settlement.Interface
	var swapService *swap.Service

	if o.SwapEnable {
		swapProtocol := swapprotocol.New(p2ps, logger, overlayEthAddress)
		swapAddressBook := swap.NewAddressbook(stateStore)
		swapService = swap.New(swapProtocol, logger, stateStore, chequebookService, chequeStore, swapAddressBook, networkID, cashoutService, p2ps)
		swapProtocol.SetSwap(swapService)
		if err = p2ps.AddProtocol(swapProtocol.Protocol()); err != nil {
			return nil, fmt.Errorf("swap protocol: %w", err)
		}
		settlement = swapService
	} else {
		pseudosettleService := pseudosettle.New(p2ps, logger, stateStore)
		if err = p2ps.AddProtocol(pseudosettleService.Protocol()); err != nil {
			return nil, fmt.Errorf("pseudosettle service: %w", err)
		}
		settlement = pseudosettleService
	}

	pricing := pricing.New(p2ps, logger, o.PaymentThreshold)
	if err = p2ps.AddProtocol(pricing.Protocol()); err != nil {
		return nil, fmt.Errorf("pricing service: %w", err)
	}

	acc, err := accounting.NewAccounting(o.PaymentThreshold, o.PaymentTolerance, o.PaymentEarly, logger, stateStore, settlement, pricing)
	if err != nil {
		return nil, fmt.Errorf("accounting: %w", err)
	}

	settlement.SetPaymentObserver(acc)
	pricing.SetPaymentThresholdObserver(acc)

	kad := kademlia.New(swarmAddress, addressbook, hive, p2ps, logger, kademlia.Options{Bootnodes: bootnodes, Standalone: o.Standalone})
	b.topologyCloser = kad
	hive.SetAddPeersHandler(kad.AddPeers)
	p2ps.SetNotifier(kad)
	addrs, err := p2ps.Addresses()
	if err != nil {
		return nil, fmt.Errorf("get server addresses: %w", err)
	}

	for _, addr := range addrs {
		logger.Debugf("p2p address: %s", addr)
	}

	var path string

	if o.DataDir != "" {
		path = filepath.Join(o.DataDir, "localstore")
	}
	lo := &localstore.Options{
		Capacity: o.DBCapacity,
	}
	storer, err := localstore.New(path, swarmAddress.Bytes(), lo, logger)
	if err != nil {
		return nil, fmt.Errorf("localstore: %w", err)
	}
	b.localstoreCloser = storer

	chunkvalidator := swarm.NewChunkValidator(content.NewValidator(), soc.NewValidator())

	retrieve := retrieval.New(swarmAddress, storer, p2ps, kad, logger, acc, accounting.NewFixedPricer(swarmAddress, 10), chunkvalidator, tracer)
	tagService := tags.NewTags(stateStore, logger)
	b.tagsCloser = tagService

	if err = p2ps.AddProtocol(retrieve.Protocol()); err != nil {
		return nil, fmt.Errorf("retrieval service: %w", err)
	}

	// instantiate the pss object
	swarmPrivateKey, _, err := keystore.Key("swarm", o.Password)
	if err != nil {
		return nil, fmt.Errorf("swarm key: %w", err)
	}

	pssService := pss.New(swarmPrivateKey, logger)
	b.pssCloser = pssService

	traversalService := traversal.NewService(storer)

	var ns storage.Storer
	if o.GlobalPinningEnabled {
		// create recovery callback for content repair
		recoverFunc := recovery.NewRecoveryHook(pssService)
		ns = netstore.New(storer, recoverFunc, retrieve, logger, chunkvalidator)
	} else {
		ns = netstore.New(storer, nil, retrieve, logger, chunkvalidator)
	}

	pushSyncProtocol := pushsync.New(p2ps, storer, kad, tagService, pssService.TryUnwrap, logger, acc, accounting.NewFixedPricer(swarmAddress, 10), tracer)

	// set the pushSyncer in the PSS
	pssService.SetPushSyncer(pushSyncProtocol)

	if err = p2ps.AddProtocol(pushSyncProtocol.Protocol()); err != nil {
		return nil, fmt.Errorf("pushsync service: %w", err)
	}

	if o.GlobalPinningEnabled {
		// register function for chunk repair upon receiving a trojan message
		chunkRepairHandler := recovery.NewRepairHandler(ns, logger, pushSyncProtocol)
		b.recoveryHandleCleanup = pssService.Register(recovery.RecoveryTopic, chunkRepairHandler)
	}

	pushSyncPusher := pusher.New(storer, kad, pushSyncProtocol, tagService, logger, tracer)
	b.pusherCloser = pushSyncPusher

	pullStorage := pullstorage.New(storer)

	pullSync := pullsync.New(p2ps, pullStorage, logger)
	b.pullSyncCloser = pullSync

	if err = p2ps.AddProtocol(pullSync.Protocol()); err != nil {
		return nil, fmt.Errorf("pullsync protocol: %w", err)
	}

	puller := puller.New(stateStore, kad, pullSync, logger, puller.Options{})

	b.pullerCloser = puller

	multiResolver := multiresolver.NewMultiResolver(
		multiresolver.WithConnectionConfigs(o.ResolverConnectionCfgs),
		multiresolver.WithLogger(o.Logger),
	)
	b.resolverCloser = multiResolver

	var apiService api.Service
	if o.APIAddr != "" {
		// API server
		apiService = api.New(tagService, ns, multiResolver, pssService, traversalService, logger, tracer, api.Options{
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

	if o.DebugAPIAddr != "" {
		// Debug API server

		debugAPIService := debugapi.New(swarmAddress, publicKey, overlayEthAddress, p2ps, pingPong, kad, storer, logger, tracer, tagService, acc, settlement, o.SwapEnable, swapService, chequebookService)
		// register metrics from components
		debugAPIService.MustRegisterMetrics(p2ps.Metrics()...)
		debugAPIService.MustRegisterMetrics(pingPong.Metrics()...)
		debugAPIService.MustRegisterMetrics(acc.Metrics()...)
		debugAPIService.MustRegisterMetrics(storer.Metrics()...)
		debugAPIService.MustRegisterMetrics(puller.Metrics()...)
		debugAPIService.MustRegisterMetrics(pushSyncProtocol.Metrics()...)
		debugAPIService.MustRegisterMetrics(pushSyncPusher.Metrics()...)
		debugAPIService.MustRegisterMetrics(pullSync.Metrics()...)

		if pssServiceMetrics, ok := pssService.(metrics.Collector); ok {
			debugAPIService.MustRegisterMetrics(pssServiceMetrics.Metrics()...)
		}

		if apiService != nil {
			debugAPIService.MustRegisterMetrics(apiService.Metrics()...)
		}
		if l, ok := logger.(metrics.Collector); ok {
			debugAPIService.MustRegisterMetrics(l.Metrics()...)
		}

		if l, ok := settlement.(metrics.Collector); ok {
			debugAPIService.MustRegisterMetrics(l.Metrics()...)
		}

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

	if err := kad.Start(p2pCtx); err != nil {
		return nil, err
	}

	return b, nil
}

func (b *Bee) Shutdown(ctx context.Context) error {
	errs := new(multiError)

	if b.apiCloser != nil {
		if err := b.apiCloser.Close(); err != nil {
			errs.add(fmt.Errorf("api: %w", err))
		}
	}

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
		errs.add(err)
	}

	if b.recoveryHandleCleanup != nil {
		b.recoveryHandleCleanup()
	}

	if err := b.pusherCloser.Close(); err != nil {
		errs.add(fmt.Errorf("pusher: %w", err))
	}

	if err := b.pullerCloser.Close(); err != nil {
		errs.add(fmt.Errorf("puller: %w", err))
	}

	if err := b.pullSyncCloser.Close(); err != nil {
		errs.add(fmt.Errorf("pull sync: %w", err))
	}

	if err := b.pssCloser.Close(); err != nil {
		errs.add(fmt.Errorf("pss: %w", err))
	}

	b.p2pCancel()
	if err := b.p2pService.Close(); err != nil {
		errs.add(fmt.Errorf("p2p server: %w", err))
	}

	if err := b.tracerCloser.Close(); err != nil {
		errs.add(fmt.Errorf("tracer: %w", err))
	}

	if err := b.tagsCloser.Close(); err != nil {
		errs.add(fmt.Errorf("tag persistence: %w", err))
	}

	if err := b.stateStoreCloser.Close(); err != nil {
		errs.add(fmt.Errorf("statestore: %w", err))
	}

	if err := b.localstoreCloser.Close(); err != nil {
		errs.add(fmt.Errorf("localstore: %w", err))
	}

	if err := b.topologyCloser.Close(); err != nil {
		errs.add(fmt.Errorf("topology driver: %w", err))
	}

	if err := b.errorLogWriter.Close(); err != nil {
		errs.add(fmt.Errorf("error log writer: %w", err))
	}

	// Shutdown the resolver service only if it has been initialized.
	if b.resolverCloser != nil {
		if err := b.resolverCloser.Close(); err != nil {
			errs.add(fmt.Errorf("resolver service: %w", err))
		}
	}

	if errs.hasErrors() {
		return errs
	}

	return nil
}

type multiError struct {
	errors []error
}

func (e *multiError) Error() string {
	if len(e.errors) == 0 {
		return ""
	}
	s := e.errors[0].Error()
	for _, err := range e.errors[1:] {
		s += "; " + err.Error()
	}
	return s
}

func (e *multiError) add(err error) {
	e.errors = append(e.errors, err)
}

func (e *multiError) hasErrors() bool {
	return len(e.errors) > 0
}
