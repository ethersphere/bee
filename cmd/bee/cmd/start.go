package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/janos/bee/pkg/api"
	"github.com/janos/bee/pkg/p2p/libp2p"
	"github.com/janos/bee/pkg/pingpong"
)

func (c *command) initStartCmd() (err error) {

	const (
		optionNameAPIAddr        = "api-addr"
		optionNameP2PAddr        = "p2p-addr"
		optionNameP2PDisableWS   = "p2p-disable-ws"
		optionNameP2PDisableQUIC = "p2p-disable-quic"
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

			h := api.New(api.Options{
				P2P:      p2ps,
				Pingpong: pingPong,
			})

			l, err := net.Listen("tcp", c.config.GetString(optionNameAPIAddr))
			if err != nil {
				return fmt.Errorf("listen TCP: %w", err)
			}

			cmd.Println("http address:", l.Addr())

			return http.Serve(l, h)
		},
	}

	cmd.Flags().String(optionNameAPIAddr, ":8500", "HTTP API listen address")
	cmd.Flags().String(optionNameP2PAddr, ":30399", "P2P listen address")
	cmd.Flags().Bool(optionNameP2PDisableWS, false, "disable P2P WebSocket protocol")
	cmd.Flags().Bool(optionNameP2PDisableQUIC, false, "disable P2P QUIC protocol")
	cmd.Flags().StringSlice(optionNameBootnodes, nil, "initial nodes to connect to")

	if err := c.config.BindPFlags(cmd.Flags()); err != nil {
		return err
	}

	c.root.AddCommand(cmd)
	return nil
}
