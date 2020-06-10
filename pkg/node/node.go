// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/hive"
	"github.com/ethersphere/bee/pkg/kademlia"
	"github.com/ethersphere/bee/pkg/keystore"
	filekeystore "github.com/ethersphere/bee/pkg/keystore/file"
	memkeystore "github.com/ethersphere/bee/pkg/keystore/mem"
	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/metrics"
	"github.com/ethersphere/bee/pkg/netstore"
	"github.com/ethersphere/bee/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/pusher"
	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/statestore/leveldb"
	mockinmem "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/ethersphere/bee/pkg/validator"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

type Bee struct {
	p2pService       io.Closer
	p2pCancel        context.CancelFunc
	apiServer        *http.Server
	debugAPIServer   *http.Server
	errorLogWriter   *io.PipeWriter
	tracerCloser     io.Closer
	stateStoreCloser io.Closer
	localstoreCloser io.Closer
	topologyCloser   io.Closer
	pusherCloser     io.Closer
}

type Options struct {
	DataDir            string
	DBCapacity         uint64
	Password           string
	APIAddr            string
	DebugAPIAddr       string
	Addr               string
	DisableWS          bool
	DisableQUIC        bool
	NetworkID          uint64
	Bootnodes          []string
	Logger             logging.Logger
	TracingEnabled     bool
	TracingEndpoint    string
	TracingServiceName string
}

func NewBee(o Options) (*Bee, error) {
	logger := o.Logger

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

	var keyStore keystore.Service
	if o.DataDir == "" {
		keyStore = memkeystore.New()
		logger.Warning("data directory not provided, keys are not persisted")
	} else {
		keyStore = filekeystore.New(filepath.Join(o.DataDir, "keys"))
	}

	swarmPrivateKey, created, err := keyStore.Key("swarm", o.Password)
	if err != nil {
		return nil, fmt.Errorf("swarm key: %w", err)
	}
	address := crypto.NewOverlayAddress(swarmPrivateKey.PublicKey, o.NetworkID)
	if created {
		logger.Info("new swarm key created")
	}

	logger.Infof("address: %s", address)

	// Construct P2P service.
	libp2pPrivateKey, created, err := keyStore.Key("libp2p", o.Password)
	if err != nil {
		return nil, fmt.Errorf("libp2p key: %w", err)
	}
	if created {
		logger.Infof("new libp2p key created")
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
	signer := crypto.NewDefaultSigner(swarmPrivateKey)

	p2ps, err := libp2p.New(p2pCtx, signer, o.NetworkID, address, o.Addr, libp2p.Options{
		PrivateKey:  libp2pPrivateKey,
		DisableWS:   o.DisableWS,
		DisableQUIC: o.DisableQUIC,
		Addressbook: addressbook,
		Logger:      logger,
		Tracer:      tracer,
	})
	if err != nil {
		return nil, fmt.Errorf("p2p service: %w", err)
	}
	b.p2pService = p2ps

	// Construct protocols.
	pingPong := pingpong.New(pingpong.Options{
		Streamer: p2ps,
		Logger:   logger,
		Tracer:   tracer,
	})

	if err = p2ps.AddProtocol(pingPong.Protocol()); err != nil {
		return nil, fmt.Errorf("pingpong service: %w", err)
	}

	hive := hive.New(hive.Options{
		Streamer:    p2ps,
		AddressBook: addressbook,
		NetworkID:   o.NetworkID,
		Logger:      logger,
	})

	if err = p2ps.AddProtocol(hive.Protocol()); err != nil {
		return nil, fmt.Errorf("hive service: %w", err)
	}

	topologyDriver := kademlia.New(kademlia.Options{Base: address, Discovery: hive, AddressBook: addressbook, P2P: p2ps, Logger: logger})
	b.topologyCloser = topologyDriver
	hive.SetPeerAddedHandler(topologyDriver.AddPeer)
	p2ps.SetNotifier(topologyDriver)
	addrs, err := p2ps.Addresses()
	if err != nil {
		return nil, fmt.Errorf("get server addresses: %w", err)
	}

	for _, addr := range addrs {
		logger.Infof("p2p address: %s", addr)
	}

	var (
		storer storage.Storer
		path   = ""
	)

	if o.DataDir != "" {
		path = filepath.Join(o.DataDir, "localstore")
	}
	lo := &localstore.Options{
		Capacity: o.DBCapacity,
	}
	storer, err = localstore.New(path, address.Bytes(), lo, logger)
	if err != nil {
		return nil, fmt.Errorf("localstore: %w", err)
	}
	b.localstoreCloser = storer

	retrieve := retrieval.New(retrieval.Options{
		Streamer:    p2ps,
		ChunkPeerer: topologyDriver,
		Logger:      logger,
	})
	tag := tags.NewTags()

	if err = p2ps.AddProtocol(retrieve.Protocol()); err != nil {
		return nil, fmt.Errorf("retrieval service: %w", err)
	}

	ns := netstore.New(storer, retrieve, validator.NewContentAddressValidator())

	retrieve.SetStorer(ns)

	pushSyncProtocol := pushsync.New(pushsync.Options{
		Streamer:      p2ps,
		Storer:        storer,
		ClosestPeerer: topologyDriver,
		Logger:        logger,
	})

	if err = p2ps.AddProtocol(pushSyncProtocol.Protocol()); err != nil {
		return nil, fmt.Errorf("pushsync service: %w", err)
	}

	pushSyncPusher := pusher.New(pusher.Options{
		Storer:        storer,
		PeerSuggester: topologyDriver,
		PushSyncer:    pushSyncProtocol,
		Tags:          tag,
		Logger:        logger,
	})
	b.pusherCloser = pushSyncPusher

	var apiService api.Service
	if o.APIAddr != "" {
		// API server
		apiService = api.New(api.Options{
			Pingpong: pingPong,
			Tags:     tag,
			Storer:   ns,
			Logger:   logger,
			Tracer:   tracer,
		})
		apiListener, err := net.Listen("tcp", o.APIAddr)
		if err != nil {
			return nil, fmt.Errorf("api listener: %w", err)
		}

		apiServer := &http.Server{
			Handler:  apiService,
			ErrorLog: log.New(b.errorLogWriter, "", 0),
		}

		go func() {
			logger.Infof("api address: %s", apiListener.Addr())

			if err := apiServer.Serve(apiListener); err != nil && err != http.ErrServerClosed {
				logger.Debugf("api server: %v", err)
				logger.Error("unable to serve api")
			}
		}()

		b.apiServer = apiServer
	}

	if o.DebugAPIAddr != "" {
		// Debug API server
		debugAPIService := debugapi.New(debugapi.Options{
			Overlay:        address,
			P2P:            p2ps,
			Logger:         logger,
			Addressbook:    addressbook,
			TopologyDriver: topologyDriver,
			Storer:         storer,
		})
		// register metrics from components
		debugAPIService.MustRegisterMetrics(p2ps.Metrics()...)
		debugAPIService.MustRegisterMetrics(pingPong.Metrics()...)
		if apiService != nil {
			debugAPIService.MustRegisterMetrics(apiService.Metrics()...)
		}
		if l, ok := logger.(metrics.Collector); ok {
			debugAPIService.MustRegisterMetrics(l.Metrics()...)
		}

		debugAPIListener, err := net.Listen("tcp", o.DebugAPIAddr)
		if err != nil {
			return nil, fmt.Errorf("debug api listener: %w", err)
		}

		debugAPIServer := &http.Server{
			Handler:  debugAPIService,
			ErrorLog: log.New(b.errorLogWriter, "", 0),
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

	overlays, err := addressbook.Overlays()
	if err != nil {
		return nil, fmt.Errorf("addressbook overlays: %w", err)
	}

	var count int32
	var wg sync.WaitGroup
	jobsC := make(chan struct{}, 16)
	for _, o := range overlays {
		jobsC <- struct{}{}
		wg.Add(1)
		go func(overlay swarm.Address) {
			defer func() {
				<-jobsC
			}()

			defer wg.Done()
			if err := topologyDriver.AddPeer(p2pCtx, overlay); err != nil {
				logger.Debugf("topology add peer fail %s: %v", overlay, err)
				logger.Errorf("topology add peer %s", overlay)
				return
			}

			atomic.AddInt32(&count, 1)
		}(o)
	}

	wg.Wait()

	// Connect bootnodes if no nodes from the addressbook was sucesufully added to topology
	if count == 0 {
		for _, a := range o.Bootnodes {
			wg.Add(1)
			go func(a string) {
				defer wg.Done()
				addr, err := ma.NewMultiaddr(a)
				if err != nil {
					logger.Debugf("multiaddress fail %s: %v", a, err)
					logger.Errorf("connect to bootnode %s", a)
					return
				}

				bzzAddr, err := p2ps.Connect(p2pCtx, addr)
				if err != nil {
					logger.Debugf("connect fail %s: %v", a, err)
					logger.Errorf("connect to bootnode %s", a)
					return
				}

				err = addressbook.Put(bzzAddr.Overlay, *bzzAddr)
				if err != nil {
					_ = p2ps.Disconnect(bzzAddr.Overlay)
					logger.Debugf("addressbook error persisting %s %s: %v", a, bzzAddr.Overlay, err)
					logger.Errorf("connect to bootnode %s", a)
					return
				}

				if err := topologyDriver.Connected(p2pCtx, bzzAddr.Overlay); err != nil {
					_ = p2ps.Disconnect(bzzAddr.Overlay)
					logger.Debugf("topology connected fail %s %s: %v", a, bzzAddr.Overlay, err)
					logger.Errorf("connect to bootnode %s", a)
					return
				}
			}(a)
		}

		wg.Wait()
	}

	return b, nil
}

func (b *Bee) Shutdown(ctx context.Context) error {
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
		return err
	}

	if err := b.pusherCloser.Close(); err != nil {
		return fmt.Errorf("pusher: %w", err)
	}

	b.p2pCancel()
	if err := b.p2pService.Close(); err != nil {
		return fmt.Errorf("p2p server: %w", err)
	}

	if err := b.tracerCloser.Close(); err != nil {
		return fmt.Errorf("tracer: %w", err)
	}

	if err := b.stateStoreCloser.Close(); err != nil {
		return fmt.Errorf("statestore: %w", err)
	}

	if err := b.localstoreCloser.Close(); err != nil {
		return fmt.Errorf("localstore: %w", err)
	}

	if err := b.topologyCloser.Close(); err != nil {
		return fmt.Errorf("topology driver: %w", err)
	}

	return b.errorLogWriter.Close()
}
