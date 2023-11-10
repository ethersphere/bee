// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"errors"
	"fmt"
	"net"
	"strings"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

type staticAddressResolver struct {
	multiProto string
	port       string
}

func newStaticAddressResolver(addr string, lookupIP func(host string) ([]net.IP, error)) (*staticAddressResolver, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	var multiProto string
	if host != "" {
		multiProto, err = getMultiProto(host, lookupIP)
		if err != nil {
			return nil, err
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

	var elements []string
	if r.multiProto != "" {
		elements = append(elements, r.multiProto)
	} else {
		elements = append(elements, observedAddrSplit[:3]...)
	}

	elements = append(elements, observedAddrSplit[3])

	if r.port != "" {
		elements = append(elements, r.port)
	} else {
		elements = append(elements, observedAddrSplit[4])
	}

	if len(observedAddrSplit) > 5 {
		elements = append(elements, observedAddrSplit[5:]...)
	}

	a, err := ma.NewMultiaddr(strings.Join(elements, "/"))
	if err != nil {
		return nil, err
	}

	return buildUnderlayAddress(a, observableAddrInfo.ID)
}

func getMultiProto(host string, lookupIP func(host string) ([]net.IP, error)) (string, error) {
	if host == "" {
		return "", nil
	}
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.To4() == nil {
			return "/ip6/" + ip.String(), nil
		}
		return "/ip4/" + ip.String(), nil
	}
	ips, err := lookupIP(host)
	if err != nil {
		return "", fmt.Errorf("invalid IP or Domain Name %q", host)
	}
	ipv4, ipv6 := ipsClassifier(ips)
	if ipv4 {
		if ipv6 {
			return "/dns/" + host, nil
		}
		return "/dns4/" + host, nil
	}
	return "/dns6/" + host, nil
}

func ipsClassifier(ips []net.IP) (ipv4, ipv6 bool) {
	for _, ip := range ips {
		if ip.To4() != nil {
			ipv4 = true
		} else {
			ipv6 = true
		}
		if ipv4 && ipv6 {
			return
		}
	}
	return
}
