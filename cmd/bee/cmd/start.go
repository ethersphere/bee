// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"context"
	"errors"
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
	"github.com/ethersphere/bee/pkg/swarm"
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
		optionNameNATAddr            = "nat-addr"
		optionNameP2PWSEnable        = "p2p-ws-enable"
		optionNameP2PQUICEnable      = "p2p-quic-enable"
		optionNameDebugAPIEnable     = "debug-api-enable"
		optionNameDebugAPIAddr       = "debug-api-addr"
		optionNameBootnodes          = "bootnode"
		optionNameNetworkID          = "network-id"
		optionWelcomeMessage         = "welcome-message"
		optionCORSAllowedOrigins     = "cors-allowed-origins"
		optionNameTracingEnabled     = "tracing-enable"
		optionNameTracingEndpoint    = "tracing-endpoint"
		optionNameTracingServiceName = "tracing-service-name"
		optionNameVerbosity          = "verbosity"
		optionNamePaymentThreshold   = "payment-threshold"
		optionNamePaymentTolerance   = "payment-tolerance"
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
			bee := `
Welcome to the Swarm.... Bzzz Bzzzz Bzzzz
                \     /                
            \    o ^ o    /            
              \ (     ) /              
   ____________(%%%%%%%)____________   
  (     /   /  )%%%%%%%(  \   \     )  
  (___/___/__/           \__\___\___)  
     (     /  /(%%%%%%%)\  \     )     
      (__/___/ (%%%%%%%) \___\__)      
              /(       )\              
            /   (%%%%%)   \            
                 (%%%)                 
                   !                   `

			fmt.Println(bee)

			debugAPIAddr := c.config.GetString(optionNameDebugAPIAddr)
			if !c.config.GetBool(optionNameDebugAPIEnable) {
				debugAPIAddr = ""
			}

			paymentThreshold := c.config.GetUint64(optionNamePaymentThreshold)
			paymentTolerance := c.config.GetUint64(optionNamePaymentTolerance)

			if paymentTolerance > paymentThreshold/2 {
				return errors.New("payment tolerance must be less than half the payment threshold")
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
				NATAddr:            c.config.GetString(optionNameNATAddr),
				EnableWS:           c.config.GetBool(optionNameP2PWSEnable),
				EnableQUIC:         c.config.GetBool(optionNameP2PQUICEnable),
				NetworkID:          c.config.GetUint64(optionNameNetworkID),
				WelcomeMessage:     c.config.GetString(optionWelcomeMessage),
				Bootnodes:          c.config.GetStringSlice(optionNameBootnodes),
				CORSAllowedOrigins: c.config.GetStringSlice(optionCORSAllowedOrigins),
				TracingEnabled:     c.config.GetBool(optionNameTracingEnabled),
				TracingEndpoint:    c.config.GetString(optionNameTracingEndpoint),
				TracingServiceName: c.config.GetString(optionNameTracingServiceName),
				Logger:             logger,
				PaymentThreshold:   paymentThreshold,
				PaymentTolerance:   paymentTolerance,
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
	cmd.Flags().Uint64(optionNameDBCapacity, 5000000, fmt.Sprintf("db capacity in chunks, multiply by %d to get approximate capacity in bytes", swarm.ChunkSize))
	cmd.Flags().String(optionNamePassword, "", "password for decrypting keys")
	cmd.Flags().String(optionNamePasswordFile, "", "path to a file that contains password for decrypting keys")
	cmd.Flags().String(optionNameAPIAddr, ":8080", "HTTP API listen address")
	cmd.Flags().String(optionNameP2PAddr, ":7070", "P2P listen address")
	cmd.Flags().String(optionNameNATAddr, "", "NAT exposed address")
	cmd.Flags().Bool(optionNameP2PWSEnable, false, "enable P2P WebSocket transport")
	cmd.Flags().Bool(optionNameP2PQUICEnable, false, "enable P2P QUIC transport")
	cmd.Flags().StringSlice(optionNameBootnodes, []string{"/dnsaddr/bootnode.ethswarm.org"}, "initial nodes to connect to")
	cmd.Flags().Bool(optionNameDebugAPIEnable, false, "enable debug HTTP API")
	cmd.Flags().String(optionNameDebugAPIAddr, ":6060", "debug HTTP API listen address")
	cmd.Flags().Uint64(optionNameNetworkID, 1, "ID of the Swarm network")
	cmd.Flags().StringSlice(optionCORSAllowedOrigins, []string{}, "origins with CORS headers enabled")
	cmd.Flags().Bool(optionNameTracingEnabled, false, "enable tracing")
	cmd.Flags().String(optionNameTracingEndpoint, "127.0.0.1:6831", "endpoint to send tracing data")
	cmd.Flags().String(optionNameTracingServiceName, "bee", "service name identifier for tracing")
	cmd.Flags().String(optionNameVerbosity, "info", "log verbosity level 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=trace")
	cmd.Flags().String(optionWelcomeMessage, "", "send a welcome message string during handshakes")
	cmd.Flags().Uint64(optionNamePaymentThreshold, 100000, "threshold in BZZ where you expect to get paid from your peers")
	cmd.Flags().Uint64(optionNamePaymentTolerance, 10000, "excess debt above payment threshold in BZZ where you disconnect from your peer")

	c.root.AddCommand(cmd)
	return nil
}
