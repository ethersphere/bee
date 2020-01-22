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

	"github.com/spf13/cobra"

	"github.com/janos/bee/pkg/api"
	"github.com/janos/bee/pkg/debugapi"
	"github.com/janos/bee/pkg/p2p/libp2p"
	"github.com/janos/bee/pkg/pingpong"
)

func (c *command) initStartCmd() (err error) {

	const (
		optionNameAPIAddr        = "api-addr"
		optionNameP2PAddr        = "p2p-addr"
		optionNameP2PDisableWS   = "p2p-disable-ws"
		optionNameP2PDisableQUIC = "p2p-disable-quic"
		optionNameDebugAPIAddr   = "debug-api-addr"
		optionNameBootnodes      = "bootnode"
	)

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a Swarm node",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if len(args) > 0 {
				return cmd.Help()
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			//var idht *dht.IpfsDHT

			// Construct P2P service.
			p2ps, err := libp2p.New(ctx, libp2p.Options{
				Addr:        c.config.GetString(optionNameP2PAddr),
				DisableWS:   c.config.GetBool(optionNameP2PDisableWS),
				DisableQUIC: c.config.GetBool(optionNameP2PDisableQUIC),
				Bootnodes:   c.config.GetStringSlice(optionNameBootnodes),
				// Routing: func(h host.Host) (r routing.PeerRouting, err error) {
				// 	idht, err = dht.New(ctx, h)
				// 	return idht, err
				// },
			})
			if err != nil {
				return fmt.Errorf("p2p service: %w", err)
			}

			// Construct protocols.
			pingPong := pingpong.New(p2ps)

			// Add protocols to the P2P service.
			if err = p2ps.AddProtocol(pingPong.Protocol()); err != nil {
				return fmt.Errorf("pingpong service: %w", err)
			}

			addrs, err := p2ps.Addresses()
			if err != nil {
				return fmt.Errorf("get server addresses: %w", err)
			}

			for _, addr := range addrs {
				cmd.Println(addr)
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
			apiServer := &http.Server{Handler: apiService}

			go func() {
				cmd.Println("api address:", apiListener.Addr())

				if err := apiServer.Serve(apiListener); err != nil && err != http.ErrServerClosed {
					log.Println("api server:", err)
				}
			}()

			var debugAPIServer *http.Server
			if addr := c.config.GetString(optionNameDebugAPIAddr); addr != "" {
				// Debug API server
				debugAPIService := debugapi.New(debugapi.Options{})
				// register metrics from components
				debugAPIService.MustRegisterMetrics(pingPong.Metrics()...)
				debugAPIService.MustRegisterMetrics(apiService.Metrics()...)

				debugAPIListener, err := net.Listen("tcp", addr)
				if err != nil {
					return fmt.Errorf("debug api listener: %w", err)
				}

				debugAPIServer := &http.Server{Handler: debugAPIService}

				go func() {
					cmd.Println("debug api address:", debugAPIListener.Addr())

					if err := debugAPIServer.Serve(debugAPIListener); err != nil && err != http.ErrServerClosed {
						log.Println("debug api server:", err)
					}
				}()
			}

			// Wait for termination or interrupt signals.
			// We want to clean up things at the end.
			interruptChannel := make(chan os.Signal, 1)
			signal.Notify(interruptChannel, syscall.SIGINT, syscall.SIGTERM)

			// Block main goroutine until it is interrupted
			sig := <-interruptChannel

			log.Println("received signal:", sig)

			// Shutdown
			done := make(chan struct{})
			go func() {
				defer func() {
					if err := recover(); err != nil {
						log.Println("shutdown panic:", err)
					}
				}()
				defer close(done)

				ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
				defer cancel()

				if err := apiServer.Shutdown(ctx); err != nil {
					log.Println("api server shutdown:", err)
				}

				if debugAPIServer != nil {
					if err := debugAPIServer.Shutdown(ctx); err != nil {
						log.Println("debug api server shutdown:", err)
					}
				}

				if err := p2ps.Close(); err != nil {
					log.Println("p2p server shutdown:", err)
				}
			}()

			// If shutdown function is blocking too long,
			// allow process termination by receiving another signal.
			select {
			case sig := <-interruptChannel:
				log.Printf("received signal: %v\n", sig)
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

	if err := c.config.BindPFlags(cmd.Flags()); err != nil {
		return err
	}

	c.root.AddCommand(cmd)
	return nil
}
