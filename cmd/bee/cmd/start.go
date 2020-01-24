// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/janos/bee/pkg/logging"
	"github.com/janos/bee/pkg/node"
	"github.com/janos/bee/pkg/p2p/libp2p"
)

func (c *command) initStartCmd() (err error) {

	const (
		optionNameDataDir          = "data-dir"
		optionNameAPIAddr          = "api-addr"
		optionNameP2PAddr          = "p2p-addr"
		optionNameP2PDisableWS     = "p2p-disable-ws"
		optionNameP2PDisableQUIC   = "p2p-disable-quic"
		optionNameDebugAPIAddr     = "debug-api-addr"
		optionNameBootnodes        = "bootnode"
		optionNameNetworkID        = "network-id"
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

			var libp2pPrivateKey io.ReadWriteCloser
			if dataDir := c.config.GetString(optionNameDataDir); dataDir != "" {
				dir := filepath.Join(dataDir, "libp2p")
				if err := os.MkdirAll(dir, os.ModePerm); err != nil {
					return err
				}
				f, err := os.OpenFile(filepath.Join(dir, "private.key"), os.O_CREATE|os.O_RDWR, 0600)
				if err != nil {
					return err
				}
				libp2pPrivateKey = f
			}

			b, err := node.NewBee(node.Options{
				APIAddr:      c.config.GetString(optionNameAPIAddr),
				DebugAPIAddr: c.config.GetString(optionNameDebugAPIAddr),
				LibP2POptions: libp2p.Options{
					PrivateKey:       libp2pPrivateKey,
					Addr:             c.config.GetString(optionNameP2PAddr),
					DisableWS:        c.config.GetBool(optionNameP2PDisableWS),
					DisableQUIC:      c.config.GetBool(optionNameP2PDisableQUIC),
					Bootnodes:        c.config.GetStringSlice(optionNameBootnodes),
					NetworkID:        c.config.GetInt(optionNameNetworkID),
					ConnectionsLow:   c.config.GetInt(optionNameConnectionsLow),
					ConnectionsHigh:  c.config.GetInt(optionNameConnectionsHigh),
					ConnectionsGrace: c.config.GetDuration(optionNameConnectionsGrace),
				},
				Logger: logger,
			})
			if err != nil {
				return err
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
				defer close(done)

				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()

				if err := b.Shutdown(ctx); err != nil {
					logger.Errorf("shutdown: %v", err)
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

	cmd.Flags().String(optionNameDataDir, filepath.Join(baseDir, "data"), "data directory")
	cmd.Flags().String(optionNameAPIAddr, ":8500", "HTTP API listen address")
	cmd.Flags().String(optionNameP2PAddr, ":30399", "P2P listen address")
	cmd.Flags().Bool(optionNameP2PDisableWS, false, "disable P2P WebSocket protocol")
	cmd.Flags().Bool(optionNameP2PDisableQUIC, false, "disable P2P QUIC protocol")
	cmd.Flags().StringSlice(optionNameBootnodes, nil, "initial nodes to connect to")
	cmd.Flags().String(optionNameDebugAPIAddr, "", "debug HTTP API listen address, e.g. 127.0.0.1:6060")
	cmd.Flags().Int(optionNameNetworkID, 1, "ID of the Swarm network")
	cmd.Flags().Int(optionNameConnectionsLow, 200, "low watermark governing the number of connections that'll be maintained")
	cmd.Flags().Int(optionNameConnectionsHigh, 400, "high watermark governing the number of connections that'll be maintained")
	cmd.Flags().Duration(optionNameConnectionsGrace, time.Minute, "the amount of time a newly opened connection is given before it becomes subject to pruning")

	if err := c.config.BindPFlags(cmd.Flags()); err != nil {
		return err
	}

	c.root.AddCommand(cmd)
	return nil
}
