// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package node defines the concept of a Bee node
// by bootstrapping and injecting all necessary
// dependencies.
package node

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/resolver/multiresolver"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/util/syncutil"
	"github.com/hashicorp/go-multierror"
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
	EnableWSS                     bool
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
