// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"context"
	"errors"
	"math/rand"
	"time"

	ma "github.com/multiformats/go-multiaddr"
	madns "github.com/multiformats/go-multiaddr-dns"
)

func resolveAddr(ctx context.Context, addr ma.Multiaddr) ([]ma.Multiaddr, error) {
	ctx, cancelFunc := context.WithTimeout(ctx, 10*time.Second)
	defer cancelFunc()
	// used to store resolved multiaddresses (ip4, ip6, dns4, dns6)
	var addresses []ma.Multiaddr
	// used to store dnsaddr multiaddresses
	var midAddrs []ma.Multiaddr
	dnsResolver := madns.DefaultResolver
	addrs, err := dnsResolver.Resolve(ctx, addr)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, errors.New("non-resolvable API endpoint")
	}
	for {
		for _, addr := range addrs {
			comp, _ := ma.SplitFirst(addr)
			if comp.Protocol().Name != "dnsaddr" {
				addresses = append(addresses, addr)
			} else {
				// not to DoS DNS server
				time.Sleep(100 * time.Millisecond)
				resAddrs, err := dnsResolver.Resolve(ctx, addr)
				if err != nil {
					// if at some point DNS fails return all resolved addresses
					if len(addresses) > 0 {
						return addresses, nil
					}
					return nil, err
				}
				for _, resAddr := range resAddrs {
					comp, _ := ma.SplitFirst(resAddr)
					if comp.Protocol().Name != "dnsaddr" {
						addresses = append(addresses, resAddr)
					} else {
						midAddrs = append(midAddrs, resAddr)
					}
				}
			}
		}
		// if no more addresses to resolve shuffle result and return
		if len(midAddrs) == 0 {
			ranAddresses := make([]ma.Multiaddr, len(addresses))
			rand.Seed(time.Now().UnixNano())
			perm := rand.Perm(len(addresses))
			for i, v := range perm {
				ranAddresses[v] = addresses[i]
			}
			return ranAddresses, nil
		}
		addrs = midAddrs
		midAddrs = nil
	}
}
