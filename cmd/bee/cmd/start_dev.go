// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ethersphere/bee/v2/pkg/node"
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
)

func (c *command) initStartDevCmd() (err error) {

	cmd := &cobra.Command{
		Use:               "dev",
		Short:             "Start a Swarm node in development mode",
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

			if c.isWindowsService {
				var err error
				logger, err = createWindowsEventLogger(serviceName, logger)
				if err != nil {
					return fmt.Errorf("failed to create windows logger %w", err)
				}
			}

			beeASCII := `
 (                      *        )  (
 )\ )                 (  *    ( /(  )\ )
(()/(   (    (   (    )\))(   )\())(()/(   (
 /(_))  )\   )\  )\  ((_)()\ ((_)\  /(_))  )\
(_))_  ((_) ((_)((_) (_()((_)  ((_)(_))_  ((_)
 |   \ | __|\ \ / /  |  \/  | / _ \ |   \ | __|
 | |) || _|  \ V /   | |\/| || (_) || |) || _|
 |___/ |___|  \_/    |_|  |_| \___/ |___/ |___|
`

			fmt.Println(beeASCII)
			fmt.Println()
			fmt.Println("Starting in development mode")
			fmt.Println()

			// generate signer in here
			b, err := node.NewDevBee(logger, &node.DevOptions{
				APIAddr:                  c.config.GetString(optionNameAPIAddr),
				Logger:                   logger,
				DBOpenFilesLimit:         c.config.GetUint64(optionNameDBOpenFilesLimit),
				DBBlockCacheCapacity:     c.config.GetUint64(optionNameDBBlockCacheCapacity),
				DBWriteBufferSize:        c.config.GetUint64(optionNameDBWriteBufferSize),
				DBDisableSeeksCompaction: c.config.GetBool(optionNameDBDisableSeeksCompaction),
				CORSAllowedOrigins:       c.config.GetStringSlice(optionCORSAllowedOrigins),
				ReserveCapacity:          c.config.GetUint64(optionNameDevReserveCapacity),
			})
			if err != nil {
				return err
			}

			// Wait for termination or interrupt signals.
			// We want to clean up things at the end.
			interruptChannel := make(chan os.Signal, 1)
			signal.Notify(interruptChannel, syscall.SIGINT, syscall.SIGTERM)

			p := &program{
				start: func() {
					// Block main goroutine until it is interrupted
					sig := <-interruptChannel

					logger.Debug("signal received", "signal", sig)
					logger.Info("shutting down")
				},
				stop: func() {
					// Shutdown
					done := make(chan struct{})
					go func() {
						defer close(done)
						if err := b.Shutdown(); err != nil {
							logger.Error(err, "shutdown failed")
						}
					}()

					// If shutdown function is blocking too long,
					// allow process termination by receiving another signal.
					select {
					case sig := <-interruptChannel:
						logger.Debug("signal received", "signal", sig)
					case <-done:
					}
				},
			}

			if c.isWindowsService {
				s, err := service.New(p, &service.Config{
					Name:        serviceName,
					DisplayName: "Bee",
					Description: "Bee, Swarm client.",
				})
				if err != nil {
					return err
				}

				if err = s.Run(); err != nil {
					return err
				}
			} else {
				// start blocks until some interrupt is received
				p.start()
				p.stop()
			}

			return nil
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return c.config.BindPFlags(cmd.Flags())
		},
	}

	cmd.Flags().String(optionNameAPIAddr, "127.0.0.1:1633", "HTTP API listen address")
	cmd.Flags().String(optionNameVerbosity, "info", "log verbosity level 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=trace")
	cmd.Flags().Uint64(optionNameDevReserveCapacity, 4194304, "cache reserve capacity")
	cmd.Flags().StringSlice(optionCORSAllowedOrigins, []string{}, "origins with CORS headers enabled")
	cmd.Flags().Uint64(optionNameDBOpenFilesLimit, 200, "number of open files allowed by database")
	cmd.Flags().Uint64(optionNameDBBlockCacheCapacity, 32*1024*1024, "size of block cache of the database in bytes")
	cmd.Flags().Uint64(optionNameDBWriteBufferSize, 32*1024*1024, "size of the database write buffer in bytes")
	cmd.Flags().Bool(optionNameDBDisableSeeksCompaction, false, "disables db compactions triggered by seeks")

	c.root.AddCommand(cmd)
	return nil
}
