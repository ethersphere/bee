// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"

	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ethersphere/bee/pkg/node"
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
)

func (c *command) initStartDevCmd() (err error) {

	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Start a Swarm node in development mode",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if len(args) > 0 {
				return cmd.Help()
			}

			v := strings.ToLower(c.config.GetString(optionNameVerbosity))
			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %v", err)
			}

			isWindowsService, err := isWindowsService()
			if err != nil {
				return fmt.Errorf("failed to determine if we are running in service: %w", err)
			}

			if isWindowsService {
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

			debugAPIAddr := c.config.GetString(optionNameDebugAPIAddr)
			if !c.config.GetBool(optionNameDebugAPIEnable) {
				debugAPIAddr = ""
			}

			// generate signer in here
			b, err := node.NewDevBee(logger, &node.DevOptions{
				APIAddr:                  c.config.GetString(optionNameAPIAddr),
				DebugAPIAddr:             debugAPIAddr,
				Logger:                   logger,
				DBOpenFilesLimit:         c.config.GetUint64(optionNameDBOpenFilesLimit),
				DBBlockCacheCapacity:     c.config.GetUint64(optionNameDBBlockCacheCapacity),
				DBWriteBufferSize:        c.config.GetUint64(optionNameDBWriteBufferSize),
				DBDisableSeeksCompaction: c.config.GetBool(optionNameDBDisableSeeksCompaction),
				CORSAllowedOrigins:       c.config.GetStringSlice(optionCORSAllowedOrigins),
				ReserveCapacity:          c.config.GetUint64(optionNameDevReserveCapacity),
				Restricted:               c.config.GetBool(optionNameRestrictedAPI),
				TokenEncryptionKey:       c.config.GetString(optionNameTokenEncryptionKey),
				AdminPasswordHash:        c.config.GetString(optionNameAdminPasswordHash),
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

					logger.Debugf("received signal: %v", sig)
					logger.Info("shutting down")
				},
				stop: func() {
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
				},
			}

			if isWindowsService {
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

	cmd.Flags().Bool(optionNameDebugAPIEnable, true, "enable debug HTTP API")
	cmd.Flags().String(optionNameAPIAddr, ":1633", "HTTP API listen address")
	cmd.Flags().String(optionNameDebugAPIAddr, ":1635", "debug HTTP API listen address")
	cmd.Flags().String(optionNameVerbosity, "info", "log verbosity level 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=trace")
	cmd.Flags().Uint64(optionNameDevReserveCapacity, 4194304, "cache reserve capacity")
	cmd.Flags().StringSlice(optionCORSAllowedOrigins, []string{}, "origins with CORS headers enabled")
	cmd.Flags().Uint64(optionNameDBOpenFilesLimit, 200, "number of open files allowed by database")
	cmd.Flags().Uint64(optionNameDBBlockCacheCapacity, 32*1024*1024, "size of block cache of the database in bytes")
	cmd.Flags().Uint64(optionNameDBWriteBufferSize, 32*1024*1024, "size of the database write buffer in bytes")
	cmd.Flags().Bool(optionNameDBDisableSeeksCompaction, false, "disables db compactions triggered by seeks")
	cmd.Flags().Bool(optionNameRestrictedAPI, false, "enable permission check on the http APIs")
	cmd.Flags().String(optionNameTokenEncryptionKey, "", "security token encryption hash")
	cmd.Flags().String(optionNameAdminPasswordHash, "", "bcrypt hash of the admin password to get the security token")

	c.root.AddCommand(cmd)
	return nil
}
