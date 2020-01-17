package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/janos/bee/pkg/p2p/libp2p"
	"github.com/janos/bee/pkg/pingpong"
	"github.com/multiformats/go-multiaddr"
)

var target = flag.String("target", "", "")

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//var idht *dht.IpfsDHT

	s, err := libp2p.New(ctx, libp2p.Options{
		// Routing: func(h host.Host) (r routing.PeerRouting, err error) {
		// 	idht, err = dht.New(ctx, h)
		// 	return idht, err
		// },
	})
	if err != nil {
		log.Fatal("p2p service: ", err)
	}

	pingPong, err := pingpong.New(s)
	if err != nil {
		log.Fatal("pingpong service: ", err)
	}

	addrs, err := s.Addresses()
	if err != nil {
		log.Fatal("get server addresses: ", err)
	}

	for _, addr := range addrs {
		fmt.Println(addr)
	}

	if *target != "" {
		for i := 1; i <= 10; i++ {
			addr, err := multiaddr.NewMultiaddr(*target)
			if err != nil {
				log.Fatal("parse target address: ", err)
			}
			peerID, err := s.Connect(ctx, addr)
			if err != nil {
				log.Fatal("connect to target: ", err)
			}
			rtt, err := pingPong.Ping(ctx, peerID, "hey", "there", ",", "how are", "you", "?")
			if err != nil {
				log.Fatal("ping target: ", err)
			}
			fmt.Println("RTT", i, rtt)
		}
	}

	select {}
}
