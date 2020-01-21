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
		optionNameListen = "listen"
	)

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a Swarm node",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			//var idht *dht.IpfsDHT

			// Construct P2P service.
			p2ps, err := libp2p.New(ctx, libp2p.Options{
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

			l, err := net.Listen("tcp", c.config.GetString(optionNameListen))
			if err != nil {
				return fmt.Errorf("listen TCP: %w", err)
			}

			cmd.Println("http address:", l.Addr())

			return http.Serve(l, h)
		},
	}

	cmd.Flags().String(optionNameListen, ":8500", "HTTP API listen address")

	if err := c.config.BindPFlags(cmd.Flags()); err != nil {
		return err
	}

	c.root.AddCommand(cmd)
	return nil
}
