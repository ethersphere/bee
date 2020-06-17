// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

type staticAddressResolver struct {
	ipProto string
	port    string
}

func newStaticAddressResolver(addr string) (handshake.AdvertisableAddressResolver, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	var ipProto string
	if host != "" {
		ip := net.ParseIP(host)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP %q", host)
		}
		if ip.To4() != nil {
			ipProto = "/ip4/" + ip.String()
		} else {
			ipProto = "/ip6/" + ip.String()
		}
	}
	return &staticAddressResolver{
		ipProto: ipProto,
		port:    port,
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

	var ipProto string
	if r.ipProto != "" {
		ipProto = r.ipProto
	} else {
		ipProto = strings.Join(observedAddrSplit[:3], "/")
	}

	var port string
	if r.port != "" {
		port = r.port
	} else {
		port = observedAddrSplit[4]
	}
	a, err := ma.NewMultiaddr(ipProto + "/" + observedAddrSplit[3] + "/" + port)
	if err != nil {
		return nil, err
	}

	return buildUnderlayAddress(a, observableAddrInfo.ID)
}
