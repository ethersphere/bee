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

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/hive"
	"github.com/ethersphere/bee/pkg/keystore"
	filekeystore "github.com/ethersphere/bee/pkg/keystore/file"
	memkeystore "github.com/ethersphere/bee/pkg/keystore/mem"
	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/metrics"
	"github.com/ethersphere/bee/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/statestore/leveldb"
	mockinmem "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology/full"
	"github.com/ethersphere/bee/pkg/tracing"
	ma "github.com/multiformats/go-multiaddr"
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
}

type Options struct {
	DataDir            string
	Password           string
	APIAddr            string
	DebugAPIAddr       string
	Addr               string
	DisableWS          bool
	DisableQUIC        bool
	NetworkID          int32
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
	address := crypto.NewAddress(swarmPrivateKey.PublicKey)
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

	p2ps, err := libp2p.New(p2pCtx, libp2p.Options{
		PrivateKey:  libp2pPrivateKey,
		Overlay:     address,
		Addr:        o.Addr,
		DisableWS:   o.DisableWS,
		DisableQUIC: o.DisableQUIC,
		NetworkID:   o.NetworkID,
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
		Logger:      logger,
	})

	if err = p2ps.AddProtocol(hive.Protocol()); err != nil {
		return nil, fmt.Errorf("hive service: %w", err)
	}

	topologyDriver := full.New(hive, addressbook, p2ps, logger, address)
	hive.SetPeerAddedHandler(topologyDriver.AddPeer)
	p2ps.SetPeerAddedHandler(topologyDriver.AddPeer)
	addrs, err := p2ps.Addresses()
	if err != nil {
		return nil, fmt.Errorf("get server addresses: %w", err)
	}

	for _, addr := range addrs {
		logger.Infof("p2p address: %s", addr)
	}

	var storer storage.Storer
	if o.DataDir == "" {
		storer, err = localstore.New("", address.Bytes(), nil, logger)
		if err != nil {
			return nil, fmt.Errorf("localstore: %w", err)
		}
	} else {
		storer, err = localstore.New(filepath.Join(o.DataDir, "localstore"), address.Bytes(), nil, logger)
		if err != nil {
			return nil, fmt.Errorf("localstore: %w", err)
		}
	}
	b.localstoreCloser = storer

	var apiService api.Service
	if o.APIAddr != "" {
		// API server
		apiService = api.New(api.Options{
			Pingpong: pingPong,
			Storer:   storer,
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

	// Connect bootnodes
	var wg sync.WaitGroup
	for _, a := range o.Bootnodes {
		wg.Add(1)
		go func(aa string) {
			defer wg.Done()
			addr, err := ma.NewMultiaddr(aa)
			if err != nil {
				logger.Debugf("multiaddress fail %s: %v", aa, err)
				logger.Errorf("connect to bootnode %s", aa)
				return
			}

			overlay, err := p2ps.Connect(p2pCtx, addr)
			if err != nil {
				logger.Debugf("connect fail %s: %v", aa, err)
				logger.Errorf("connect to bootnode %s", aa)
				return
			}

			err = addressbook.Put(overlay, addr)
			if err != nil {
				_ = p2ps.Disconnect(overlay)
				logger.Debugf("addressboook error persisting %s %s: %v", aa, overlay, err)
				logger.Errorf("persisting node %s", aa)
				return

			}

			if err := topologyDriver.AddPeer(p2pCtx, overlay); err != nil {
				_ = p2ps.Disconnect(overlay)
				logger.Debugf("topology add peer fail %s %s: %v", aa, overlay, err)
				logger.Errorf("connect to bootnode %s", aa)
				return
			}
		}(a)
	}

	wg.Wait()

	overlays, err := addressbook.Overlays()
	if err != nil {
		return nil, fmt.Errorf("addressbook overlays: %w", err)
	}

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
				_ = p2ps.Disconnect(overlay)
				logger.Debugf("topology add peer fail %s: %v", overlay, err)
				logger.Errorf("topology add peer %s", overlay)
				return
			}
		}(o)
	}

	wg.Wait()

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

	return b.errorLogWriter.Close()
}
