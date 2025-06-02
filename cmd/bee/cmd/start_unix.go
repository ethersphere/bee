// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ethersphere/bee/v2"
	"github.com/ethersphere/bee/v2/pkg/node"
	"github.com/spf13/cobra"
)

func (c *command) initStartCmd() (err error) {
	cmd := &cobra.Command{
		Use:               "start",
		Short:             "Start a Swarm node",
		PersistentPreRunE: c.CheckUnknownParams,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if len(args) > 0 {
				return cmd.Help()
			}

			v := strings.ToLower(c.config.GetString(optionNameVerbosity))

			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %w", err)
			}

			fmt.Print(beeWelcomeMessage)
			time.Sleep(5 * time.Second)
			fmt.Printf("\n\nversion: %v - planned to be supported until %v, please follow https://ethswarm.org/\n\n", bee.Version, endSupportDate())
			logger.Info("bee version", "version", bee.Version)

			go startTimeBomb(logger)

			// ctx is global context of bee node; which is canceled after interrupt signal is received.
			ctx, cancel := context.WithCancel(context.Background())
			sysInterruptChannel := make(chan os.Signal, 1)
			signal.Notify(sysInterruptChannel, syscall.SIGINT, syscall.SIGTERM)

			go func() {
				select {
				case <-sysInterruptChannel:
					logger.Info("received interrupt signal")
					cancel()
				case <-ctx.Done():
				}
			}()

			// Building bee node can take up some time (because node.NewBee(...) is compute have function )
			// Because of this we need to do it in background so that program could be terminated when interrupt signal is received
			// while bee node is being constructed.
			respC := buildBeeNodeAsync(ctx, c, cmd, logger)
			var beeNode atomic.Value

			p := &program{
				start: func() {
					// Wait for bee node to fully build and initialized
					select {
					case resp := <-respC:
						if resp.err != nil {
							logger.Error(resp.err, "failed to build bee node")
							return
						}
						beeNode.Store(resp.bee)
					case <-ctx.Done():
						return
					}

					// Bee has fully started at this point, from now on we
					// block main goroutine until it is interrupted or stopped
					select {
					case <-ctx.Done():
					case <-beeNode.Load().(*node.Bee).SyncingStopped():
						logger.Debug("syncing has stopped")
					}

					logger.Info("shutting down...")
				},
				stop: func() {
					// Whenever program is being stopped we need to cancel main context
					// beforehand so that node could be stopped via Shutdown method
					cancel()

					// Shutdown node (if node was fully started)
					val := beeNode.Load()
					if val == nil {
						return
					}

					done := make(chan struct{})
					go func(beeNode *node.Bee) {
						defer close(done)

						if err := beeNode.Shutdown(); err != nil {
							logger.Error(err, "shutdown failed")
						}
					}(val.(*node.Bee))

					// If shutdown function is blocking too long,
					// allow process termination by receiving another signal.
					select {
					case <-sysInterruptChannel:
						logger.Info("node shutdown terminated")
					case <-done:
						logger.Info("node shutdown")
					}
				},
			}

			// start blocks until some interrupt is received
			p.start()
			p.stop()

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
