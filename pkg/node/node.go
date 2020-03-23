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

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/ethersphere/bee/pkg/addressbook/inmem"
	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/hive"
	"github.com/ethersphere/bee/pkg/keystore"
	filekeystore "github.com/ethersphere/bee/pkg/keystore/file"
	memkeystore "github.com/ethersphere/bee/pkg/keystore/mem"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/metrics"
	"github.com/ethersphere/bee/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/topology/full"
	ma "github.com/multiformats/go-multiaddr"
)

type Bee struct {
	p2pService     io.Closer
	p2pCancel      context.CancelFunc
	apiServer      *http.Server
	debugAPIServer *http.Server
	errorLogWriter *io.PipeWriter
}

type Options struct {
	DataDir       string
	Password      string
	APIAddr       string
	DebugAPIAddr  string
	LibP2POptions libp2p.Options
	Bootnodes     []string
	Logger        logging.Logger
}

func NewBee(o Options) (*Bee, error) {
	logger := o.Logger

	p2pCtx, p2pCancel := context.WithCancel(context.Background())

	b := &Bee{
		p2pCancel:      p2pCancel,
		errorLogWriter: logger.WriterLevel(logrus.ErrorLevel),
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

	libP2POptions := o.LibP2POptions
	libP2POptions.Overlay = address
	libP2POptions.PrivateKey = libp2pPrivateKey
	p2ps, err := libp2p.New(p2pCtx, libP2POptions)
	if err != nil {
		return nil, fmt.Errorf("p2p service: %w", err)
	}
	b.p2pService = p2ps

	// TODO: be more resilient on connection errors and connect in parallel
	for _, a := range o.Bootnodes {
		addr, err := ma.NewMultiaddr(a)
		if err != nil {
			return nil, fmt.Errorf("bootnode %s: %w", a, err)
		}

		overlay, err := p2ps.Connect(p2pCtx, addr)
		if err != nil {
			return nil, fmt.Errorf("connect to bootnode %s %s: %w", a, overlay, err)
		}
	}

	addressbook := inmem.New()

	// Construct protocols.
	pingPong := pingpong.New(pingpong.Options{
		Streamer: p2ps,
		Logger:   logger,
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

	topologyDriver := full.New(hive, addressbook, p2ps, logger)
	hive.AddPeerAddedHandler(topologyDriver.PeerAddedHandler)
	p2ps.SetPeerAddedHandler(topologyDriver.PeerAddedHandler)
	addrs, err := p2ps.Addresses()
	if err != nil {
		return nil, fmt.Errorf("get server addresses: %w", err)
	}

	for _, addr := range addrs {
		logger.Infof("p2p address: %s", addr)
	}

	var apiService api.Service
	if o.APIAddr != "" {
		// API server
		apiService = api.New(api.Options{
			Pingpong: pingPong,
			Logger:   logger,
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

	return b.errorLogWriter.Close()
}
