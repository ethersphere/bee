// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/metrics"
	"github.com/ethersphere/bee/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/pkg/pingpong"
)

type Bee struct {
	p2pService     io.Closer
	p2pCancel      context.CancelFunc
	apiServer      *http.Server
	debugAPIServer *http.Server
	errorLogWriter *io.PipeWriter
}

type Options struct {
	PrivateKey    io.ReadWriteCloser
	APIAddr       string
	DebugAPIAddr  string
	LibP2POptions libp2p.Options
	Logger        logging.Logger
}

func NewBee(o Options) (*Bee, error) {
	logger := o.Logger

	p2pCtx, p2pCancel := context.WithCancel(context.Background())

	b := &Bee{
		p2pCancel:      p2pCancel,
		errorLogWriter: logger.WriterLevel(logrus.ErrorLevel),
	}

	var privateKey *ecdsa.PrivateKey
	if o.PrivateKey != nil {
		privateKeyData, err := ioutil.ReadAll(o.PrivateKey)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read private key: %w", err)
		}
		if len(privateKeyData) == 0 {
			var err error
			privateKey, err = crypto.GenerateSecp256k1Key()
			if err != nil {
				return nil, fmt.Errorf("generate secp256k1 key: %w", err)
			}
			d, err := crypto.MarshalSecp256k1PrivateKey(privateKey)
			if err != nil {
				return nil, fmt.Errorf("encode private key: %w", err)
			}
			if _, err := io.Copy(o.PrivateKey, bytes.NewReader(d)); err != nil {
				return nil, fmt.Errorf("write private key: %w", err)
			}
		} else {
			var err error
			privateKey, err = crypto.UnmarshalPrivateKey(privateKeyData)
			if err != nil {
				return nil, fmt.Errorf("decode private key: %w", err)
			}
		}
		if err := o.PrivateKey.Close(); err != nil {
			return nil, fmt.Errorf("close private key: %w", err)
		}
	} else {
		var err error
		privateKey, err = crypto.GenerateSecp256k1Key()
		if err != nil {
			return nil, fmt.Errorf("generate secp256k1 key: %w", err)
		}
	}

	address := crypto.NewAddress(privateKey.PublicKey)
	logger.Infof("address: %s", address)

	// Construct P2P service.
	libP2POptions := o.LibP2POptions
	libP2POptions.Overlay = address
	p2ps, err := libp2p.New(p2pCtx, libP2POptions)
	if err != nil {
		return nil, fmt.Errorf("p2p service: %w", err)
	}
	b.p2pService = p2ps

	// Construct protocols.
	pingPong := pingpong.New(pingpong.Options{
		Streamer: p2ps,
		Logger:   logger,
	})

	// Add protocols to the P2P service.
	if err = p2ps.AddProtocol(pingPong.Protocol()); err != nil {
		return nil, fmt.Errorf("pingpong service: %w", err)
	}

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
			P2P:    p2ps,
			Logger: logger,
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
