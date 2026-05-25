// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/config"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/node"
	statestoremock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
)

func TestValidatePublicAddress(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		addr   string
		expErr bool
	}{
		{
			name:   "empty host",
			addr:   ":1635",
			expErr: true,
		},
		{
			name:   "localhost",
			addr:   "localhost:1635",
			expErr: true,
		},
		{
			name:   "loopback",
			addr:   "127.0.0.1:1635",
			expErr: true,
		},
		{
			name:   "loopback ipv6",
			addr:   "[::1]:1635",
			expErr: true,
		},
		{
			name:   "missing port",
			addr:   "1.2.3.4",
			expErr: true,
		},
		{
			name:   "empty port",
			addr:   "1.2.3.4:",
			expErr: true,
		},
		{
			name:   "invalid port number",
			addr:   "1.2.3.4:abc",
			expErr: true,
		},
		{
			name:   "valid",
			addr:   "1.2.3.4:1635",
			expErr: false,
		},
		{
			name:   "valid ipv6",
			addr:   "[2001:db8::1]:1635",
			expErr: false,
		},
		{
			name:   "empty",
			addr:   "",
			expErr: false,
		},
		{
			name:   "valid hostname",
			addr:   "example.com:8080",
			expErr: false,
		},
		{
			name:   "valid hostname with hyphen",
			addr:   "test-example.com:8080",
			expErr: false,
		},
		{
			name:   "private IP",
			addr:   "192.168.1.1:8080",
			expErr: true,
		},
		{
			name:   "invalid hostname format",
			addr:   "invalid..hostname:8080",
			expErr: true,
		},
		{
			name:   "hostname starts with hyphen",
			addr:   "-test.com:8080",
			expErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := node.ValidatePublicAddress(tc.addr)
			if tc.expErr && err == nil {
				t.Fatal("expected error, but got none")
			}
			if !tc.expErr && err != nil {
				t.Fatalf("expected no error, but got: %v", err)
			}
		})
	}
}

// validBaseOptions returns an Options value that passes validateOptions. Tests
// mutate one field at a time to assert exactly which check fires.
func validBaseOptions() node.Options {
	return node.Options{
		FullNodeMode:     true,
		PaymentThreshold: "10000000", // sits inside [minPaymentThreshold, maxPaymentThreshold]
		PaymentTolerance: 0,
		PaymentEarly:     0,
	}
}

func TestValidateOptions(t *testing.T) {
	t.Parallel()

	// maxAllowedDoubling is an unexported constant in pkg/node; mirror its value
	// here so a deliberate change to the limit forces a deliberate test update.
	const maxAllowedDoubling = 1

	mutate := func(f func(*node.Options)) node.Options {
		o := validBaseOptions()
		f(&o)
		return o
	}

	testCases := []struct {
		name    string
		opts    node.Options
		wantErr string
	}{
		{
			name: "all valid full node",
			opts: validBaseOptions(),
		},
		{
			name: "all valid light node",
			opts: mutate(func(o *node.Options) { o.FullNodeMode = false }),
		},
		{
			name:    "invalid NAT address",
			opts:    mutate(func(o *node.Options) { o.NATAddr = "localhost:1635" }),
			wantErr: "invalid NAT address",
		},
		{
			name:    "invalid NAT WSS address",
			opts:    mutate(func(o *node.Options) { o.NATWSSAddr = "127.0.0.1:1635" }),
			wantErr: "invalid NAT WSS address",
		},
		{
			name: "light node with non-zero doubling",
			opts: mutate(func(o *node.Options) {
				o.FullNodeMode = false
				o.ReserveCapacityDoubling = 1
			}),
			wantErr: "reserve capacity doubling is only allowed for full nodes",
		},
		{
			name:    "doubling above max",
			opts:    mutate(func(o *node.Options) { o.ReserveCapacityDoubling = maxAllowedDoubling + 1 }),
			wantErr: "reserve capacity doubling has to be between",
		},
		{
			name:    "doubling negative",
			opts:    mutate(func(o *node.Options) { o.ReserveCapacityDoubling = -1 }),
			wantErr: "reserve capacity doubling has to be between",
		},
		{
			name:    "payment threshold empty",
			opts:    mutate(func(o *node.Options) { o.PaymentThreshold = "" }),
			wantErr: "invalid payment threshold",
		},
		{
			name:    "payment threshold non-numeric",
			opts:    mutate(func(o *node.Options) { o.PaymentThreshold = "abc" }),
			wantErr: "invalid payment threshold",
		},
		{
			name:    "payment threshold below minimum",
			opts:    mutate(func(o *node.Options) { o.PaymentThreshold = "1" }),
			wantErr: "payment threshold below minimum",
		},
		{
			name:    "payment threshold above maximum",
			opts:    mutate(func(o *node.Options) { o.PaymentThreshold = "999999999999" }),
			wantErr: "payment threshold above maximum",
		},
		{
			name:    "payment tolerance negative",
			opts:    mutate(func(o *node.Options) { o.PaymentTolerance = -1 }),
			wantErr: "invalid payment tolerance",
		},
		{
			name:    "payment early negative",
			opts:    mutate(func(o *node.Options) { o.PaymentEarly = -1 }),
			wantErr: "invalid payment early",
		},
		{
			name:    "payment early above 100",
			opts:    mutate(func(o *node.Options) { o.PaymentEarly = 101 }),
			wantErr: "invalid payment early",
		},
		{
			name: "payment early at boundary 100",
			opts: mutate(func(o *node.Options) { o.PaymentEarly = 100 }),
		},
		{
			name:    "target neighborhood invalid bitstring",
			opts:    mutate(func(o *node.Options) { o.TargetNeighborhood = "10X01" }),
			wantErr: "invalid neighborhood",
		},
		{
			name: "target neighborhood valid bitstring",
			opts: mutate(func(o *node.Options) { o.TargetNeighborhood = "101010" }),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := node.ValidateOptions(&tc.opts)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestParsePaymentThreshold(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		input   string
		wantErr string
		want    string // string form of the expected bigint, only checked on success
	}{
		{name: "empty parses to zero, fails below minimum", input: "", wantErr: "invalid payment threshold"},
		{name: "non-numeric", input: "nope", wantErr: "invalid payment threshold"},
		{name: "below minimum", input: "1", wantErr: "below minimum"},
		{name: "above maximum", input: "999999999999", wantErr: "above maximum"},
		{name: "at minimum", input: "9000000", want: "9000000"},
		{name: "at maximum", input: "108000000", want: "108000000"},
		{name: "inside range", input: "10000000", want: "10000000"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := node.ParsePaymentThreshold(tc.input)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.String() != tc.want {
				t.Fatalf("parsed %q -> %s, want %s", tc.input, got.String(), tc.want)
			}
		})
	}
}

func TestValidateChainContractOptions(t *testing.T) {
	t.Parallel()

	const (
		hexAddr = "0x1234567890123456789012345678901234567890"
		bogus   = "not-a-hex-address"
	)
	// 99999999 is not testnet or mainnet, so config.GetByChainID returns
	// found=false for it.
	const unknownChain int64 = 99999999

	testCases := []struct {
		name    string
		opts    node.Options
		chainID int64
		wantErr string
	}{
		{
			name:    "known chain, no custom postage address is OK",
			opts:    node.Options{},
			chainID: config.Mainnet.ChainID,
		},
		{
			name:    "unknown chain without custom postage address",
			opts:    node.Options{},
			chainID: unknownChain,
			wantErr: "no known postage stamp addresses for this network",
		},
		{
			name:    "custom postage address malformed",
			opts:    node.Options{PostageContractAddress: bogus, PostageContractStartBlock: 1},
			chainID: unknownChain,
			wantErr: "malformed postage stamp address",
		},
		{
			name:    "custom postage address without start block",
			opts:    node.Options{PostageContractAddress: hexAddr, PostageContractStartBlock: 0},
			chainID: unknownChain,
			wantErr: "postage contract start block option not provided",
		},
		{
			name:    "custom postage address with start block on unknown chain is OK",
			opts:    node.Options{PostageContractAddress: hexAddr, PostageContractStartBlock: 1},
			chainID: unknownChain,
		},
		{
			name:    "staking address malformed",
			opts:    node.Options{StakingContractAddress: bogus},
			chainID: config.Mainnet.ChainID,
			wantErr: "malformed staking contract address",
		},
		{
			name:    "staking address valid",
			opts:    node.Options{StakingContractAddress: hexAddr},
			chainID: config.Mainnet.ChainID,
		},
		{
			name:    "redistribution address malformed but incentives off — silently accepted",
			opts:    node.Options{RedistributionContractAddress: bogus, EnableStorageIncentives: false},
			chainID: config.Mainnet.ChainID,
		},
		{
			name:    "redistribution address malformed with incentives on",
			opts:    node.Options{RedistributionContractAddress: bogus, EnableStorageIncentives: true},
			chainID: config.Mainnet.ChainID,
			wantErr: "malformed redistribution contract address",
		},
		{
			name:    "redistribution address valid with incentives on",
			opts:    node.Options{RedistributionContractAddress: hexAddr, EnableStorageIncentives: true},
			chainID: config.Mainnet.ChainID,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := node.ValidateChainContractOptions(&tc.opts, tc.chainID)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestIsChainEnabled(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		fullNode     bool
		swapEndpoint string
		want         bool
	}{
		{name: "light no endpoint disables chain", fullNode: false, swapEndpoint: "", want: false},
		{name: "light with endpoint enables chain", fullNode: false, swapEndpoint: "http://rpc", want: true},
		{name: "full no endpoint enables chain", fullNode: true, swapEndpoint: "", want: true},
		{name: "full with endpoint enables chain", fullNode: true, swapEndpoint: "http://rpc", want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := node.IsChainEnabled(&node.Options{FullNodeMode: tc.fullNode}, tc.swapEndpoint, log.Noop)
			if got != tc.want {
				t.Fatalf("isChainEnabled(fullNode=%v, swapEndpoint=%q) = %v, want %v",
					tc.fullNode, tc.swapEndpoint, got, tc.want)
			}
		})
	}
}

func TestBatchStoreExists(t *testing.T) {
	t.Parallel()

	t.Run("empty store", func(t *testing.T) {
		t.Parallel()
		s := statestoremock.NewStateStore()
		got, err := node.BatchStoreExists(s)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Fatal("expected false on an empty store")
		}
	})

	t.Run("unrelated key only", func(t *testing.T) {
		t.Parallel()
		s := statestoremock.NewStateStore()
		if err := s.Put("not_batchstore_key", []byte("v")); err != nil {
			t.Fatalf("put: %v", err)
		}
		got, err := node.BatchStoreExists(s)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Fatal("expected false when no batchstore_ prefixed key is present")
		}
	})

	t.Run("batchstore key present", func(t *testing.T) {
		t.Parallel()
		s := statestoremock.NewStateStore()
		if err := s.Put("batchstore_foo", []byte("v")); err != nil {
			t.Fatalf("put: %v", err)
		}
		got, err := node.BatchStoreExists(s)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got {
			t.Fatal("expected true when a batchstore_ prefixed key is present")
		}
	})

	t.Run("iterate error surfaces", func(t *testing.T) {
		t.Parallel()
		want := errors.New("boom")
		got, err := node.BatchStoreExists(&iterErrStore{err: want})
		if !errors.Is(err, want) {
			t.Fatalf("expected %v, got %v", want, err)
		}
		if got {
			t.Fatal("expected false when iterate returns an error")
		}
	})
}

// iterErrStore is a StateStorer that returns a fixed error from Iterate.
// batchStoreExists only ever calls Iterate, so the other methods are stubbed.
type iterErrStore struct {
	err error
}

var _ storage.StateStorer = (*iterErrStore)(nil)

func (s *iterErrStore) Iterate(_ string, _ storage.StateIterFunc) error { return s.err }
func (s *iterErrStore) Get(string, any) error                           { return nil }
func (s *iterErrStore) Put(string, any) error                           { return nil }
func (s *iterErrStore) Delete(string) error                             { return nil }
func (s *iterErrStore) Close() error                                    { return nil }
