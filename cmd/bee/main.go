package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sync"

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

	// Construct P2P service.
	s, err := libp2p.New(ctx, libp2p.Options{
		// Routing: func(h host.Host) (r routing.PeerRouting, err error) {
		// 	idht, err = dht.New(ctx, h)
		// 	return idht, err
		// },
	})
	if err != nil {
		log.Fatal("p2p service: ", err)
	}

	// Construct protocols.
	pingPong := pingpong.New(s)

	// Add protocols to the P2P service.
	if err = s.AddProtocol(pingPong.Protocol()); err != nil {
		log.Fatal("pingpong service: ", err)
	}

	// Bellow is only demo code.

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

			var wg sync.WaitGroup
			wg.Add(2)

			go func() {
				defer wg.Done()
				rtt, err := pingPong.Ping(ctx, peerID, "hey", "there", ",", "how are", "you", "?")
				if err != nil {
					log.Fatal("ping: ", err)
				}
				fmt.Println("RTT 1", i, rtt)
			}()

			go func() {
				defer wg.Done()
				rtt, err := pingPong.Ping(ctx, peerID, "1", "2", "3", "4", "5", "6")
				if err != nil {
					log.Fatal("ping: ", err)
				}
				fmt.Println("RTT 2", i, rtt)
			}()

			wg.Wait()
		}
	}

	select {}
}
