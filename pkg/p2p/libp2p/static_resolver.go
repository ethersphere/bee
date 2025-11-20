// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"fmt"
	"net"
	"strings"

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
	observedAddrSplit := strings.Split(observedAddress.String(), "/")

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
	a, err := ma.NewMultiaddr(multiProto + "/" + observedAddrSplit[3] + "/" + port + "/" + strings.Join(observedAddrSplit[5:], "/"))
	if err != nil {
		return nil, err
	}

	return a, nil
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
