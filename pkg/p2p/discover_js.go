//go:build js

// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	madns "github.com/multiformats/go-multiaddr-dns"
)

// newDNSResolver returns a DNS resolver backed by DNS-over-HTTPS for wasm
// builds, where the system resolver is unavailable in the browser sandbox.
func newDNSResolver() (*madns.Resolver, error) {
	return madns.NewResolver(madns.WithDefaultResolver(NewCustomDNSResolver()))
}

// CustomDNSResolver implements the madns.BasicResolver interface using
// Google's DNS-over-HTTPS API, since browsers do not expose system DNS.
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
			txt := strings.Trim(answer.Data, "\"")
			txts = append(txts, txt)
		}
	}
	return txts, nil
}
