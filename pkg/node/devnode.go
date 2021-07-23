package node

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"time"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/feeds/factory"
	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pinning"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore"
	mockPost "github.com/ethersphere/bee/pkg/postage/mock"
	mockPostContract "github.com/ethersphere/bee/pkg/postage/postagecontract/mock"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/pushsync"
	mockPs "github.com/ethersphere/bee/pkg/pushsync/mock"
	"github.com/ethersphere/bee/pkg/settlement"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/statestore/leveldb"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology/lightnode"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/bee/pkg/traversal"
	"github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"

	accountingMock "github.com/ethersphere/bee/pkg/accounting/mock"
	p2pMock "github.com/ethersphere/bee/pkg/p2p/mock"
	pingpongMock "github.com/ethersphere/bee/pkg/pingpong/mock"
	topologyMock "github.com/ethersphere/bee/pkg/topology/mock"
)

func NewDevBee(logger logging.Logger, o *Options) (b *Bee, err error) {
	tracer, tracerCloser, err := tracing.NewTracer(&tracing.Options{
		Enabled:     o.TracingEnabled,
		Endpoint:    o.TracingEndpoint,
		ServiceName: o.TracingServiceName,
	})
	if err != nil {
		return nil, fmt.Errorf("tracer: %w", err)
	}

	b = &Bee{ //DevBee with custom shutdown (os.Exit(0))
		errorLogWriter: logger.WriterLevel(logrus.ErrorLevel),
		tracerCloser:   tracerCloser,
	}

	stateStore, err := leveldb.NewInMemoryStateStore(logger)
	if err != nil {
		return nil, err
	}
	b.stateStoreCloser = stateStore

	mockKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		return nil, err
	}
	signer := crypto.NewDefaultSigner(mockKey)

	var (
		transactionService transaction.Service // mock for debug api
		chequebookService  chequebook.Service  // mock both
	)

	overlayEthAddress, err := signer.EthereumAddress()
	if err != nil {
		return nil, fmt.Errorf("eth address: %w", err)
	}

	// set up basic debug api endpoints for debugging and /health endpoint
	debugAPIService := debugapi.New(mockKey.PublicKey, mockKey.PublicKey, overlayEthAddress, logger, tracer, nil, big.NewInt(0), transactionService)

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

	// var (
	// 	// TO BE MOCKED
	// 	networkID     uint64
	// 	swarmAddress  swarm.Address
	// 	lightNodes    *lightnode.Container
	// 	senderMatcher *transaction.Matcher
	// 	swapBackend   listener.BlockHeightContractFilterer
	// 	ns            storage.Storer
	// )

	// p2ps, err := libp2p.New(p2pCtx, signer, networkID, swarmAddress, addr, addressbook, stateStore, lightNodes, senderMatcher, logger, tracer, libp2p.Options{
	// 	PrivateKey:     libp2pPrivateKey,
	// 	NATAddr:        o.NATAddr,
	// 	EnableWS:       o.EnableWS,
	// 	EnableQUIC:     o.EnableQUIC,
	// 	WelcomeMessage: o.WelcomeMessage,
	// 	FullNode:       true,
	// })
	// if err != nil {
	// 	return nil, fmt.Errorf("p2p service: %w", err)
	// }
	// b.p2pService = p2ps
	// b.p2pHalter = p2ps

	// mock ^

	lo := &localstore.Options{
		Capacity:        o.CacheCapacity,
		ReserveCapacity: uint64(batchstore.Capacity),
		UnreserveFunc: func(postage.UnreserveIteratorFn) error {
			return nil
		},
		OpenFilesLimit:         o.DBOpenFilesLimit,
		BlockCacheCapacity:     o.DBBlockCacheCapacity,
		WriteBufferSize:        o.DBWriteBufferSize,
		DisableSeeksCompaction: o.DBDisableSeeksCompaction,
	}

	var swarmAddress swarm.Address
	storer, err := localstore.New("", swarmAddress.Bytes(), stateStore, lo, logger)
	if err != nil {
		return nil, fmt.Errorf("localstore: %w", err)
	}
	b.localstoreCloser = storer
	// unreserveFn = storer.UnreserveBatch

	// post, err := postage.NewService(stateStore, batchStore, 0)
	// if err != nil {
	// 	return nil, fmt.Errorf("postage service load: %w", err)
	// }

	// use mock from postage/mock
	// b.postageServiceCloser = post

	// var (
	// 	postageContractService postagecontract.Interface
	// 	eventListener          postage.Listener
	// 	postageContractAddress common.Address
	// )

	// postageContractService = new(mockContract)

	// eventListener = listener.New(logger, swapBackend, postageContractAddress, o.BlockTime, &pidKiller{node: b})
	// b.listenerCloser = eventListener

	// if natManager := p2ps.NATManager(); natManager != nil {
	// 	// wait for nat manager to init
	// 	logger.Debug("initializing NAT manager")
	// 	select {
	// 	case <-natManager.Ready():
	// 		// this is magic sleep to give NAT time to sync the mappings
	// 		// this is a hack, kind of alchemy and should be improved
	// 		time.Sleep(3 * time.Second)
	// 		logger.Debug("NAT manager initialized")
	// 	case <-time.After(10 * time.Second):
	// 		logger.Warning("NAT manager init timeout")
	// 	}
	// }

	// Construct protocols.
	// pingPong := pingpong.New(p2ps, logger, tracer)

	// if err = p2ps.AddProtocol(pingPong.Protocol()); err != nil {
	// 	return nil, fmt.Errorf("pingpong service: %w", err)
	// }

	// hive := hive.New(p2ps, addressbook, networkID, logger)
	// if err = p2ps.AddProtocol(hive.Protocol()); err != nil {
	// 	return nil, fmt.Errorf("hive service: %w", err)
	// }

	// var bootnodes []ma.Multiaddr

	// metricsDB, err := shed.NewDBWrap(stateStore.DB())
	// if err != nil {
	// 	return nil, fmt.Errorf("unable to create metrics storage for kademlia: %w", err)
	// }

	// kad := kademlia.New(swarmAddress, addressbook, hive, p2ps, metricsDB, logger, kademlia.Options{Bootnodes: bootnodes, BootnodeMode: o.BootnodeMode})
	// b.topologyCloser = kad
	// b.topologyHalter = kad
	// hive.SetAddPeersHandler(kad.AddPeers)
	// p2ps.SetPickyNotifier(kad)
	// batchStore.SetRadiusSetter(kad)

	// var (
	// 	paymentTolerance = big.NewInt(1000)
	// 	paymentEarly     = big.NewInt(1000)
	// 	paymentThreshold = big.NewInt(10000)
	// )

	// var pricing pricing.Interface

	// acc, err := accounting.NewAccounting(
	// 	paymentThreshold,
	// 	paymentTolerance,
	// 	paymentEarly,
	// 	logger,
	// 	stateStore,
	// 	pricing,
	// 	big.NewInt(refreshRate),
	// 	p2ps,
	// )
	// if err != nil {
	// 	return nil, fmt.Errorf("accounting: %w", err)
	// }
	// b.accountingCloser = acc

	// pseudosettleService := pseudosettle.New(p2ps, logger, stateStore, acc, big.NewInt(refreshRate), p2ps)
	// if err = p2ps.AddProtocol(pseudosettleService.Protocol()); err != nil {
	// 	return nil, fmt.Errorf("pseudosettle service: %w", err)
	// }

	// acc.SetRefreshFunc(pseudosettleService.Pay)

	tagService := tags.NewTags(stateStore, logger)
	b.tagsCloser = tagService

	var pssPrivateKey *ecdsa.PrivateKey
	pssService := pss.New(pssPrivateKey, logger)
	b.pssCloser = pssService

	pssService.SetPushSyncer(mockPs.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		return &pushsync.Receipt{}, nil
	}))

	traversalService := traversal.New(storer) //fixme

	pinningService := pinning.NewService(storer, stateStore, traversalService)

	batchStore, err := batchstore.New(stateStore, func(b []byte) error { return nil }, logger)
	if err != nil {
		return nil, fmt.Errorf("batchstore: %w", err)
	}
	post := mockPost.New()
	postageContract := mockPostContract.New(mockPostContract.WithCreateBatchFunc(
		func(ctx context.Context, initialBalance *big.Int, depth uint8, immutable bool, label string) ([]byte, error) {
			id := postagetesting.MustNewID()
			b := &postage.Batch{
				ID:        id,
				Owner:     overlayEthAddress.Bytes(),
				Value:     big.NewInt(0),
				Depth:     depth,
				Immutable: immutable,
			}

			err := batchStore.Put(b, initialBalance, depth)
			if err != nil {
				return nil, err
			}

			post.Add(postage.NewStampIssuer(
				label,
				string(overlayEthAddress.Bytes()),
				id,
				initialBalance,
				depth,
				0, 0,
				immutable,
			))
			return id, nil
		},
	))

	var apiService api.Service
	if o.APIAddr != "" {
		// API server
		feedFactory := factory.New(storer)

		apiService = api.New(tagService, storer, nil, pssService, traversalService, pinningService, feedFactory, post, postageContract, nil, signer, logger, tracer, api.Options{
			// mock errored ^
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

	lightNodes := lightnode.NewContainer(swarm.NewAddress(nil))

	var (
		// mock or find existing mocks
		pseudosettleService settlement.Interface
	)

	addrFunc := p2pMock.WithAddressesFunc(addrFunc)
	var (
		pingPong = pingpongMock.New(pong)
		p2ps     = p2pMock.New(addrFunc)
		acc      = accountingMock.NewAccounting()
		kad      = topologyMock.NewTopologyDriver()
	)

	// inject dependencies and configure full debug api http path routes
	debugAPIService.Configure(swarmAddress, p2ps, pingPong, kad, lightNodes, storer, tagService, acc, pseudosettleService, false, nil, chequebookService, batchStore, post, postageContract)

	return b, nil
}

func addrFunc() ([]multiaddr.Multiaddr, error) {
	ma, _ := multiaddr.NewMultiaddr("mock")
	return []multiaddr.Multiaddr{ma}, nil
}

func pong(ctx context.Context, address swarm.Address, msgs ...string) (rtt time.Duration, err error) {
	return time.Millisecond, nil
}
