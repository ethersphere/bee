package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/janos/bee/pkg/api"
	"github.com/janos/bee/pkg/p2p/libp2p"
	"github.com/janos/bee/pkg/pingpong"
)

var addr = flag.String("addr", ":0", "http api listen address")

func main() {
	flag.Parse()

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
		log.Fatal("p2p service: ", err)
	}

	// Construct protocols.
	pingPong := pingpong.New(p2ps)

	// Add protocols to the P2P service.
	if err = p2ps.AddProtocol(pingPong.Protocol()); err != nil {
		log.Fatal("pingpong service: ", err)
	}

	addrs, err := p2ps.Addresses()
	if err != nil {
		log.Fatal("get server addresses: ", err)
	}

	for _, addr := range addrs {
		fmt.Println(addr)
	}

	h := api.New(api.Options{
		P2P:      p2ps,
		Pingpong: pingPong,
	})

	l, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal("tcp: ", err)
	}

	log.Println("listening: ", l.Addr())

	log.Fatal(http.Serve(l, h))
}
