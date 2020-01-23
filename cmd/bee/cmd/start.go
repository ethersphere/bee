// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/janos/bee/pkg/api"
	"github.com/janos/bee/pkg/debugapi"
	"github.com/janos/bee/pkg/logging"
	"github.com/janos/bee/pkg/p2p/libp2p"
	"github.com/janos/bee/pkg/pingpong"
)

func (c *command) initStartCmd() (err error) {

	const (
		optionNameAPIAddr          = "api-addr"
		optionNameP2PAddr          = "p2p-addr"
		optionNameP2PDisableWS     = "p2p-disable-ws"
		optionNameP2PDisableQUIC   = "p2p-disable-quic"
		optionNameDebugAPIAddr     = "debug-api-addr"
		optionNameBootnodes        = "bootnode"
		optionNameConnectionsLow   = "connections-low"
		optionNameConnectionsHigh  = "connections-high"
		optionNameConnectionsGrace = "connections-grace"
	)

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a Swarm node",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if len(args) > 0 {
				return cmd.Help()
			}

			logger := logging.New(cmd.OutOrStdout())
			logger.SetLevel(logrus.TraceLevel)

			errorLogWriter := logger.WriterLevel(logrus.ErrorLevel)
			defer errorLogWriter.Close()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Construct P2P service.
			p2ps, err := libp2p.New(ctx, libp2p.Options{
				Addr:             c.config.GetString(optionNameP2PAddr),
				DisableWS:        c.config.GetBool(optionNameP2PDisableWS),
				DisableQUIC:      c.config.GetBool(optionNameP2PDisableQUIC),
				Bootnodes:        c.config.GetStringSlice(optionNameBootnodes),
				ConnectionsLow:   c.config.GetInt(optionNameConnectionsLow),
				ConnectionsHigh:  c.config.GetInt(optionNameConnectionsHigh),
				ConnectionsGrace: c.config.GetDuration(optionNameConnectionsGrace),
			})
			if err != nil {
				return fmt.Errorf("p2p service: %w", err)
			}

			// Construct protocols.
			pingPong := pingpong.New(pingpong.Options{
				Streamer: p2ps,
				Logger:   logger,
			})

			// Add protocols to the P2P service.
			if err = p2ps.AddProtocol(pingPong.Protocol()); err != nil {
				return fmt.Errorf("pingpong service: %w", err)
			}

			addrs, err := p2ps.Addresses()
			if err != nil {
				return fmt.Errorf("get server addresses: %w", err)
			}

			for _, addr := range addrs {
				logger.Infof("address: %s", addr)
			}

			// API server
			apiService := api.New(api.Options{
				P2P:      p2ps,
				Pingpong: pingPong,
			})
			apiListener, err := net.Listen("tcp", c.config.GetString(optionNameAPIAddr))
			if err != nil {
				return fmt.Errorf("api listener: %w", err)
			}

			apiServer := &http.Server{
				Handler:  apiService,
				ErrorLog: log.New(errorLogWriter, "", 0),
			}

			go func() {
				logger.Infof("api address: %s", apiListener.Addr())

				if err := apiServer.Serve(apiListener); err != nil && err != http.ErrServerClosed {
					logger.Errorf("api server: %v", err)
				}
			}()

			var debugAPIServer *http.Server
			if addr := c.config.GetString(optionNameDebugAPIAddr); addr != "" {
				// Debug API server
				debugAPIService := debugapi.New(debugapi.Options{})
				// register metrics from components
				debugAPIService.MustRegisterMetrics(p2ps.Metrics()...)
				debugAPIService.MustRegisterMetrics(pingPong.Metrics()...)
				debugAPIService.MustRegisterMetrics(apiService.Metrics()...)

				debugAPIListener, err := net.Listen("tcp", addr)
				if err != nil {
					return fmt.Errorf("debug api listener: %w", err)
				}

				debugAPIServer := &http.Server{
					Handler:  debugAPIService,
					ErrorLog: log.New(errorLogWriter, "", 0),
				}

				go func() {
					logger.Infof("debug api address: %s", debugAPIListener.Addr())

					if err := debugAPIServer.Serve(debugAPIListener); err != nil && err != http.ErrServerClosed {
						logger.Errorf("debug api server: %v", err)
					}
				}()
			}

			// Wait for termination or interrupt signals.
			// We want to clean up things at the end.
			interruptChannel := make(chan os.Signal, 1)
			signal.Notify(interruptChannel, syscall.SIGINT, syscall.SIGTERM)

			// Block main goroutine until it is interrupted
			sig := <-interruptChannel

			logger.Debugf("received signal: %v", sig)

			// Shutdown
			done := make(chan struct{})
			go func() {
				defer func() {
					if err := recover(); err != nil {
						logger.Errorf("shutdown panic: %v", err)
					}
				}()
				defer close(done)

				ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
				defer cancel()

				if err := apiServer.Shutdown(ctx); err != nil {
					logger.Errorf("api server shutdown: %v", err)
				}

				if debugAPIServer != nil {
					if err := debugAPIServer.Shutdown(ctx); err != nil {
						logger.Errorf("debug api server shutdown: %v", err)
					}
				}

				if err := p2ps.Close(); err != nil {
					logger.Errorf("p2p server shutdown: %v", err)
				}
			}()

			// If shutdown function is blocking too long,
			// allow process termination by receiving another signal.
			select {
			case sig := <-interruptChannel:
				logger.Debugf("received signal: %v", sig)
			case <-done:
			}

			return nil
		},
	}

	cmd.Flags().String(optionNameAPIAddr, ":8500", "HTTP API listen address")
	cmd.Flags().String(optionNameP2PAddr, ":30399", "P2P listen address")
	cmd.Flags().Bool(optionNameP2PDisableWS, false, "disable P2P WebSocket protocol")
	cmd.Flags().Bool(optionNameP2PDisableQUIC, false, "disable P2P QUIC protocol")
	cmd.Flags().StringSlice(optionNameBootnodes, nil, "initial nodes to connect to")
	cmd.Flags().String(optionNameDebugAPIAddr, ":6060", "Debug HTTP API listen address")
	cmd.Flags().Int(optionNameConnectionsLow, 200, "low watermark governing the number of connections that'll be maintained")
	cmd.Flags().Int(optionNameConnectionsHigh, 400, "high watermark governing the number of connections that'll be maintained")
	cmd.Flags().Duration(optionNameConnectionsGrace, time.Minute, "the amount of time a newly opened connection is given before it becomes subject to pruning")

	if err := c.config.BindPFlags(cmd.Flags()); err != nil {
		return err
	}

	c.root.AddCommand(cmd)
	return nil
}
