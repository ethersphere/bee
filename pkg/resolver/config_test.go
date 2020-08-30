// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package resolver_test

import (
	"errors"
	"testing"

	"github.com/ethersphere/bee/pkg/resolver"
)

func TestParseConnectionStrings(t *testing.T) {
	testCases := []struct {
		desc       string
		conStrings []string
		wantCfg    []resolver.ConnectionConfig
		wantErr    error
	}{
		{
			desc: "tld too long",
			conStrings: []string{
				"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff:example.com",
			},
			wantErr: resolver.ErrTLDTooLong,
		},
		{
			desc: "single endpoint default tld",
			conStrings: []string{
				"https://example.com",
			},
			wantCfg: []resolver.ConnectionConfig{
				{
					TLD:      "",
					Endpoint: "https://example.com",
				},
			},
		},
		{
			desc: "single endpoint explicit tld",
			conStrings: []string{
				"tld:https://example.com",
			},
			wantCfg: []resolver.ConnectionConfig{
				{
					TLD:      "tld",
					Endpoint: "https://example.com",
				},
			},
		},
		{
			desc: "single endpoint with address default tld",
			conStrings: []string{
				"0x314159265dD8dbb310642f98f50C066173C1259b@https://example.com",
			},
			wantCfg: []resolver.ConnectionConfig{
				{
					TLD:      "",
					Address:  "0x314159265dD8dbb310642f98f50C066173C1259b",
					Endpoint: "https://example.com",
				},
			},
		},
		{
			desc: "single endpoint with address explicit tld",
			conStrings: []string{
				"tld:0x314159265dD8dbb310642f98f50C066173C1259b@https://example.com",
			},
			wantCfg: []resolver.ConnectionConfig{
				{
					TLD:      "tld",
					Address:  "0x314159265dD8dbb310642f98f50C066173C1259b",
					Endpoint: "https://example.com",
				},
			},
		},
		{
			desc: "mixed",
			conStrings: []string{
				"tld:https://example.com",
				"testdomain:wowzers.map",
				"yesyesyes:0x314159265dD8dbb310642f98f50C066173C1259b@2.2.2.2",
				"cloudflare-ethereum.org",
			},
			wantCfg: []resolver.ConnectionConfig{
				{
					TLD:      "tld",
					Endpoint: "https://example.com",
				},
				{
					TLD:      "testdomain",
					Endpoint: "wowzers.map",
				},
				{
					TLD:      "yesyesyes",
					Address:  "0x314159265dD8dbb310642f98f50C066173C1259b",
					Endpoint: "2.2.2.2",
				},
				{
					TLD:      "",
					Endpoint: "cloudflare-ethereum.org",
				},
			},
		},
		{
			desc: "mixed with error",
			conStrings: []string{
				"tld:https://example.com",
				"testdomain:wowzers.map",
				"nonononononononononononononononononononononononononononononononononono:yes",
			},
			wantErr: resolver.ErrTLDTooLong,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			got, err := resolver.ParseConnectionStrings(tC.conStrings)
			if err != nil {
				if !errors.Is(err, tC.wantErr) {
					t.Errorf("got error %v", err)
				}
				return
			}

			for i, el := range got {
				want := tC.wantCfg[i]
				got := el
				if got.TLD != want.TLD {
					t.Errorf("got %q, want %q", got.TLD, want.TLD)
				}
				if got.Address != want.Address {
					t.Errorf("got %q, want %q", got.Address, want.Address)
				}
				if got.Endpoint != want.Endpoint {
					t.Errorf("got %q, want %q", got.Endpoint, want.Endpoint)
				}
			}
		})
	}
}
