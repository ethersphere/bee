// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/node"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func (c *command) initStartCmd() (err error) {

	const (
		optionNameDataDir            = "data-dir"
		optionNameDBCapacity         = "db-capacity"
		optionNamePassword           = "password"
		optionNamePasswordFile       = "password-file"
		optionNameAPIAddr            = "api-addr"
		optionNameP2PAddr            = "p2p-addr"
		optionNameP2PDisableWS       = "p2p-disable-ws"
		optionNameP2PDisableQUIC     = "p2p-disable-quic"
		optionNameEnableDebugAPI     = "enable-debug-api"
		optionNameDebugAPIAddr       = "debug-api-addr"
		optionNameBootnodes          = "bootnode"
		optionNameNetworkID          = "network-id"
		optionNameTracingEnabled     = "tracing"
		optionNameTracingEndpoint    = "tracing-endpoint"
		optionNameTracingServiceName = "tracing-service-name"
		optionNameVerbosity          = "verbosity"
	)

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a Swarm node",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if len(args) > 0 {
				return cmd.Help()
			}

			var logger logging.Logger
			switch v := strings.ToLower(c.config.GetString(optionNameVerbosity)); v {
			case "0", "silent":
				logger = logging.New(ioutil.Discard, 0)
			case "1", "error":
				logger = logging.New(cmd.OutOrStdout(), logrus.ErrorLevel)
			case "2", "warn":
				logger = logging.New(cmd.OutOrStdout(), logrus.WarnLevel)
			case "3", "info":
				logger = logging.New(cmd.OutOrStdout(), logrus.InfoLevel)
			case "4", "debug":
				logger = logging.New(cmd.OutOrStdout(), logrus.DebugLevel)
			case "5", "trace":
				logger = logging.New(cmd.OutOrStdout(), logrus.TraceLevel)
			default:
				return fmt.Errorf("unknown verbosity level %q", v)
			}

			debugAPIAddr := c.config.GetString(optionNameDebugAPIAddr)
			if !c.config.GetBool(optionNameEnableDebugAPI) {
				debugAPIAddr = ""
			}

			var password string
			if p := c.config.GetString(optionNamePassword); p != "" {
				password = p
			} else if pf := c.config.GetString(optionNamePasswordFile); pf != "" {
				b, err := ioutil.ReadFile(pf)
				if err != nil {
					return err
				}
				password = string(bytes.Trim(b, "\n"))
			} else {
				p, err := terminalPromptPassword(cmd, c.passwordReader, "Password")
				if err != nil {
					return err
				}
				password = p
			}

			b, err := node.NewBee(node.Options{
				DataDir:            c.config.GetString(optionNameDataDir),
				DBCapacity:         c.config.GetUint64(optionNameDBCapacity),
				Password:           password,
				APIAddr:            c.config.GetString(optionNameAPIAddr),
				DebugAPIAddr:       debugAPIAddr,
				Addr:               c.config.GetString(optionNameP2PAddr),
				DisableWS:          c.config.GetBool(optionNameP2PDisableWS),
				DisableQUIC:        c.config.GetBool(optionNameP2PDisableQUIC),
				NetworkID:          c.config.GetUint64(optionNameNetworkID),
				Bootnodes:          c.config.GetStringSlice(optionNameBootnodes),
				TracingEnabled:     c.config.GetBool(optionNameTracingEnabled),
				TracingEndpoint:    c.config.GetString(optionNameTracingEndpoint),
				TracingServiceName: c.config.GetString(optionNameTracingServiceName),
				Logger:             logger,
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
			logger.Info("shutting down")

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
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return c.config.BindPFlags(cmd.Flags())
		},
	}

	cmd.Flags().String(optionNameDataDir, filepath.Join(c.homeDir, ".bee"), "data directory")
	cmd.Flags().Uint64(optionNameDBCapacity, 5000000, "db capacity in chunks")
	cmd.Flags().String(optionNamePassword, "", "password for decrypting keys")
	cmd.Flags().String(optionNamePasswordFile, "", "path to a file that contains password for decrypting keys")
	cmd.Flags().String(optionNameAPIAddr, ":8080", "HTTP API listen address")
	cmd.Flags().String(optionNameP2PAddr, ":7070", "P2P listen address")
	cmd.Flags().Bool(optionNameP2PDisableWS, false, "disable P2P WebSocket protocol")
	cmd.Flags().Bool(optionNameP2PDisableQUIC, false, "disable P2P QUIC protocol")
	cmd.Flags().StringSlice(optionNameBootnodes, nil, "initial nodes to connect to")
	cmd.Flags().Bool(optionNameEnableDebugAPI, false, "enable debug HTTP API")
	cmd.Flags().String(optionNameDebugAPIAddr, ":6060", "debug HTTP API listen address")
	cmd.Flags().Uint64(optionNameNetworkID, 1, "ID of the Swarm network")
	cmd.Flags().Bool(optionNameTracingEnabled, false, "enable tracing")
	cmd.Flags().String(optionNameTracingEndpoint, "127.0.0.1:6831", "endpoint to send tracing data")
	cmd.Flags().String(optionNameTracingServiceName, "bee", "service name identifier for tracing")
	cmd.Flags().String(optionNameVerbosity, "info", "log verbosity level 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=trace")

	c.root.AddCommand(cmd)
	return nil
}
