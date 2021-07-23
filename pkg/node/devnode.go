package node

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

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
	"github.com/ethersphere/bee/pkg/statestore/leveldb"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology/lightnode"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/ethersphere/bee/pkg/traversal"

	accountingMock "github.com/ethersphere/bee/pkg/accounting/mock"
	p2pMock "github.com/ethersphere/bee/pkg/p2p/mock"
	pingpongMock "github.com/ethersphere/bee/pkg/pingpong/mock"
	topologyMock "github.com/ethersphere/bee/pkg/topology/mock"
)

type DevBee struct {
	tracerCloser     io.Closer
	stateStoreCloser io.Closer
	localstoreCloser io.Closer
	apiCloser        io.Closer
	pssCloser        io.Closer
	tagsCloser       io.Closer
	errorLogWriter   *io.PipeWriter
	apiServer        *http.Server
	debugAPIServer   *http.Server
}

func NewDevBee(logger logging.Logger, o *Options) (b *DevBee, err error) {
	tracer, tracerCloser, err := tracing.NewTracer(&tracing.Options{
		Enabled:     o.TracingEnabled,
		Endpoint:    o.TracingEndpoint,
		ServiceName: o.TracingServiceName,
	})
	if err != nil {
		return nil, fmt.Errorf("tracer: %w", err)
	}

	b = &DevBee{
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

	overlayEthAddress, err := signer.EthereumAddress()
	if err != nil {
		return nil, fmt.Errorf("eth address: %w", err)
	}

	debugAPIService := debugapi.New(mockKey.PublicKey, mockKey.PublicKey, overlayEthAddress, logger, tracer, nil, big.NewInt(0), nil)

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

	tagService := tags.NewTags(stateStore, logger)
	b.tagsCloser = tagService

	pssService := pss.New(mockKey, logger)
	b.pssCloser = pssService

	pssService.SetPushSyncer(mockPs.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		pssService.TryUnwrap(chunk)
		return &pushsync.Receipt{}, nil
	}))

	traversalService := traversal.New(storer)

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

	// API server
	feedFactory := factory.New(storer)

	apiService := api.New(tagService, storer, nil, pssService, traversalService, pinningService, feedFactory, post, postageContract, nil, signer, logger, tracer, api.Options{
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

	var (
		lightNodes = lightnode.NewContainer(swarm.NewAddress(nil))
		pingPong   = pingpongMock.New(pong)
		addrFunc   = p2pMock.WithAddressesFunc(addrFunc)
		p2ps       = p2pMock.New(addrFunc)
		acc        = accountingMock.NewAccounting()
		kad        = topologyMock.NewTopologyDriver()
	)

	// inject dependencies and configure full debug api http path routes
	debugAPIService.Configure(swarmAddress, p2ps, pingPong, kad, lightNodes, storer, tagService, acc, nil, false, nil, nil, batchStore, post, postageContract)

	return b, nil
}

func (b *DevBee) Shutdown(ctx context.Context) error {
	var mErr error

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

	tryClose(b.pssCloser, "pss")
	tryClose(b.tracerCloser, "tracer")
	tryClose(b.tagsCloser, "tag persistence")
	tryClose(b.stateStoreCloser, "statestore")
	tryClose(b.localstoreCloser, "localstore")
	tryClose(b.errorLogWriter, "error log writer")

	return mErr
}

func addrFunc() ([]multiaddr.Multiaddr, error) {
	ma, _ := multiaddr.NewMultiaddr("mock")
	return []multiaddr.Multiaddr{ma}, nil
}

func pong(ctx context.Context, address swarm.Address, msgs ...string) (rtt time.Duration, err error) {
	return time.Millisecond, nil
}
