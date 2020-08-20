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

	"strings"
	"syscall"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/node"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func (c *command) initStartCmd() (err error) {

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

			b, err := node.NewBee(c.config.GetString(optionNameP2PAddr), logger, node.Options{
				DataDir:              c.config.GetString(optionNameDataDir),
				DBCapacity:           c.config.GetUint64(optionNameDBCapacity),
				Password:             password,
				APIAddr:              c.config.GetString(optionNameAPIAddr),
				DebugAPIAddr:         debugAPIAddr,
				Addr:                 c.config.GetString(optionNameP2PAddr),
				NATAddr:              c.config.GetString(optionNameNATAddr),
				EnableWS:             c.config.GetBool(optionNameP2PWSEnable),
				EnableQUIC:           c.config.GetBool(optionNameP2PQUICEnable),
				NetworkID:            c.config.GetUint64(optionNameNetworkID),
				WelcomeMessage:       c.config.GetString(optionWelcomeMessage),
				Bootnodes:            c.config.GetStringSlice(optionNameBootnodes),
				CORSAllowedOrigins:   c.config.GetStringSlice(optionCORSAllowedOrigins),
				TracingEnabled:       c.config.GetBool(optionNameTracingEnabled),
				TracingEndpoint:      c.config.GetString(optionNameTracingEndpoint),
				TracingServiceName:   c.config.GetString(optionNameTracingServiceName),
				Logger:               logger,
				GlobalPinningEnabled: c.config.GetBool(optionNameGlobalPinningEnabled),
				PaymentThreshold:     c.config.GetUint64(optionNamePaymentThreshold),
				PaymentTolerance:     c.config.GetUint64(optionNamePaymentTolerance),
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

	c.setAllFlags(cmd)

	c.root.AddCommand(cmd)
	return nil
}
