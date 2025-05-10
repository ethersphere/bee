// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package p2p

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"

	ma "github.com/multiformats/go-multiaddr"
	madns "github.com/multiformats/go-multiaddr-dns"
)

func isDNSProtocol(protoCode int) bool {
	if protoCode == ma.P_DNS || protoCode == ma.P_DNS4 || protoCode == ma.P_DNS6 || protoCode == ma.P_DNSADDR {
		return true
	}
	return false
}

// CustomDNSResolver implements madns.BasicResolver interface
type CustomDNSResolver struct {
	client *http.Client
}

func NewCustomDNSResolver() *CustomDNSResolver {
	return &CustomDNSResolver{
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

type dnsResponse struct {
	Answer []struct {
		Type int    `json:"type"`
		Data string `json:"data"`
	} `json:"Answer"`
}

func (r *CustomDNSResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	// Use Google's DNS-over-HTTPS API
	url := fmt.Sprintf("https://dns.google/resolve?name=%s&type=A", host)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dns query: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var dnsResp dnsResponse
	if err := json.Unmarshal(body, &dnsResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	var addrs []net.IPAddr
	for _, answer := range dnsResp.Answer {
		if answer.Type == 1 { // A record
			ip := net.ParseIP(answer.Data)
			if ip != nil {
				addrs = append(addrs, net.IPAddr{IP: ip})
			}
		}
	}
	return addrs, nil
}

func (r *CustomDNSResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	// Use Google's DNS-over-HTTPS API
	url := fmt.Sprintf("https://dns.google/resolve?name=%s&type=TXT", name)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dns query: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var dnsResp dnsResponse
	if err := json.Unmarshal(body, &dnsResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	var txts []string
	for _, answer := range dnsResp.Answer {
		if answer.Type == 16 { // TXT record
			// Remove quotes from TXT record data
			txt := strings.Trim(answer.Data, "\"")
			txts = append(txts, txt)
		}
	}
	return txts, nil
}

func Discover(ctx context.Context, addr ma.Multiaddr, f func(ma.Multiaddr) (bool, error)) (bool, error) {
	if comp, _ := ma.SplitFirst(addr); !isDNSProtocol(comp.Protocol().Code) {
		return f(addr)
	}

	// Create a custom DNS resolver using Google's DNS-over-HTTPS
	customResolver := NewCustomDNSResolver()
	dnsResolver, err := madns.NewResolver(madns.WithDefaultResolver(customResolver))
	if err != nil {
		return false, fmt.Errorf("create dns resolver: %w", err)
	}
	addrs, err := dnsResolver.Resolve(ctx, addr)
	if err != nil {
		return false, fmt.Errorf("dns resolve address %s: %w", addr, err)
	}
	if len(addrs) == 0 {
		return false, errors.New("non-resolvable API endpoint")
	}

	rand.Shuffle(len(addrs), func(i, j int) {
		addrs[i], addrs[j] = addrs[j], addrs[i]
	})
	for _, addr := range addrs {
		stopped, err := Discover(ctx, addr, f)
		if err != nil {
			return false, fmt.Errorf("discover %s: %w", addr, err)
		}

		if stopped {
			return true, nil
		}
	}

	return false, nil
}
