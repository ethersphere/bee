// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"errors"
	"fmt"
	"net"
	"strings"

	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

type staticAddressResolver struct {
	multiProto string
	port       string
}

func newStaticAddressResolver(addr string) (*staticAddressResolver, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	var multiProto string
	if host != "" {
		ip := net.ParseIP(host)
		if ip == nil {
			ipv4 := false
			ipv6 := false
			ips, err := net.LookupIP(host)
			if err != nil {
				return nil, fmt.Errorf("invalid IP or Domain Name %q", host)
			}
			for _, ip := range ips {
				if ip.To4() != nil {
					ipv4 = true
				} else {
					ipv6 = true
				}
			}
			if ipv4 {
				multiProto = "/dns4/" + host
				if ipv6 {
					multiProto = "/dns/" + host
				}
			} else {
				multiProto = "/dns6/" + host
			}
		} else if ip.To4() != nil {
			multiProto = "/ip4/" + ip.String()
		} else {
			multiProto = "/ip6/" + ip.String()
		}
	}
	return &staticAddressResolver{
		multiProto: multiProto,
		port:       port,
	}, nil
}

func (r *staticAddressResolver) Resolve(observedAddress ma.Multiaddr) (ma.Multiaddr, error) {
	observableAddrInfo, err := libp2ppeer.AddrInfoFromP2pAddr(observedAddress)
	if err != nil {
		return nil, err
	}

	if len(observableAddrInfo.Addrs) < 1 {
		return nil, errors.New("invalid observed address")
	}

	observedAddrSplit := strings.Split(observableAddrInfo.Addrs[0].String(), "/")

	// if address is not in a form of '/ipversion/ip/protocol/port/...` don't compare to addresses and return it
	if len(observedAddrSplit) < 5 {
		return observedAddress, nil
	}

	var multiProto string
	if r.multiProto != "" {
		multiProto = r.multiProto
	} else {
		multiProto = strings.Join(observedAddrSplit[:3], "/")
	}

	var port string
	if r.port != "" {
		port = r.port
	} else {
		port = observedAddrSplit[4]
	}
	a, err := ma.NewMultiaddr(multiProto + "/" + observedAddrSplit[3] + "/" + port)
	if err != nil {
		return nil, err
	}

	return buildUnderlayAddress(a, observableAddrInfo.ID)
}
