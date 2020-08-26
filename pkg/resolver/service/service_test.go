// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/resolver/service"
	"github.com/syndtr/goleveldb/leveldb/errors"
)

func TestParseConnectionString(t *testing.T) {
	testCases := []struct {
		conString string
		wantCfg   service.ConnectionConfig
		wantError error
	}{
		{
			conString: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff:example.com",
			wantError: errors.New("TLD too long"),
		},
		{
			conString: "https://example.com",
			wantCfg: service.ConnectionConfig{
				TLD:      "",
				Endpoint: "https://example.com",
			},
		},
		{
			conString: "tld:https://example.com",
			wantCfg: service.ConnectionConfig{
				TLD:      "tld",
				Endpoint: "https://example.com",
			},
		},
		{
			conString: "0x314159265dD8dbb310642f98f50C066173C1259b@https://example.com",
			wantCfg: service.ConnectionConfig{
				TLD:      "",
				Address:  "0x314159265dD8dbb310642f98f50C066173C1259b",
				Endpoint: "https://example.com",
			},
		},
		{
			conString: "tld:0x314159265dD8dbb310642f98f50C066173C1259b@https://example.com",
			wantCfg: service.ConnectionConfig{
				TLD:      "tld",
				Address:  "0x314159265dD8dbb310642f98f50C066173C1259b",
				Endpoint: "https://example.com",
			},
		},
	}
	for _, tC := range testCases {
		t.Run(tC.conString, func(t *testing.T) {
			got, err := service.ParseConnectionString(tC.conString)
			if err != nil {
				if tC.wantError == nil {
					t.Errorf("got error %v", err)
				}
				return
			}
			if got.TLD != tC.wantCfg.TLD {
				t.Errorf("got %q, want %q", got.TLD, tC.wantCfg.TLD)
			}
			if got.Address != tC.wantCfg.Address {
				t.Errorf("got %q, want %q", got.Address, tC.wantCfg.Address)
			}
			if got.Endpoint != tC.wantCfg.Endpoint {
				t.Errorf("got %q, want %q", got.Endpoint, tC.wantCfg.Endpoint)
			}
		})
	}
}

func TestParseConnectionStrings(t *testing.T) {
	testCases := []struct {
		desc       string
		conStrings []string
		wantCfg    []service.ConnectionConfig
		wantErr    error
	}{
		{
			desc: "ok",
			conStrings: []string{
				"tld:https://example.com",
				"testdomain:wowzers.map",
				"yesyesyes:0x314159265dD8dbb310642f98f50C066173C1259b@2.2.2.2",
				"cloudflare-ethereum.org",
			},
			wantCfg: []service.ConnectionConfig{
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
			desc: "expected error",
			conStrings: []string{
				"tld:https://example.com",
				"testdomain:wowzers.map",
				"nonononononononononononononononononononononononononononononononononono:yes",
			},
			wantErr: errors.New("Resolver connection string: TLD extend max length of 63 characters"),
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			got, err := service.ParseConnectionStrings(tC.conStrings)
			if err != nil {
				if tC.wantErr == nil {
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
