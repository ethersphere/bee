// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ens_test

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/ethersphere/bee/pkg/resolver/client/ens"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestNewENSClient(t *testing.T) {
	testCases := []struct {
		desc         string
		endpoint     string
		dialFn       func(string) (*ethclient.Client, error)
		wantErr      error
		wantEndpoint string
	}{
		{
			desc:     "nil dial function",
			endpoint: "someaddress.net",
			dialFn:   nil,
			wantErr:  ens.ErrNotImplemented,
		},
		{
			desc:     "error in dial function",
			endpoint: "someaddress.com",
			dialFn: func(string) (*ethclient.Client, error) {
				return nil, errors.New("dial error")
			},
			wantErr: ens.ErrFailedToConnect,
		},
		{
			desc:     "regular endpoint",
			endpoint: "someaddress.org",
			dialFn: func(string) (*ethclient.Client, error) {
				return &ethclient.Client{}, nil
			},
			wantEndpoint: "someaddress.org",
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			cl, err := ens.NewClient(tC.endpoint,
				ens.WithDialFunc(tC.dialFn),
			)
			if err != nil {
				if !errors.Is(err, tC.wantErr) {
					t.Errorf("got %v, want %v", err, tC.wantErr)
				}
				return
			}
			if got := cl.Endpoint(); got != tC.wantEndpoint {
				t.Errorf("endpoint: got %v, want %v", got, tC.wantEndpoint)
			}
			if got := cl.IsConnected(); got != true {
				t.Errorf("connected: got %v, want true", got)
			}
		})
	}
}

func TestClose(t *testing.T) {
	t.Run("connected", func(t *testing.T) {
		rpcServer := rpc.NewServer()
		defer rpcServer.Stop()
		ethCl := ethclient.NewClient(rpc.DialInProc(rpcServer))

		cl, err := ens.NewClient("",
			ens.WithDialFunc(func(string) (*ethclient.Client, error) {
				return ethCl, nil
			}),
		)
		if err != nil {
			t.Fatal(err)
		}

		err = cl.Close()
		if err != nil {
			t.Fatal(err)
		}

		if cl.IsConnected() {
			t.Error("IsConnected == true")
		}
	})
	t.Run("not connected", func(t *testing.T) {
		cl, err := ens.NewClient("",
			ens.WithDialFunc(func(string) (*ethclient.Client, error) {
				return nil, nil
			}),
		)
		if err != nil {
			t.Fatal(err)
		}

		err = cl.Close()
		if err != nil {
			t.Fatal(err)
		}

		if cl.IsConnected() {
			t.Error("IsConnected == true")
		}
	})
}

func TestResolve(t *testing.T) {
	addr := swarm.MustParseHexAddress("aaabbbcc")

	testCases := []struct {
		desc      string
		name      string
		resolveFn func(bind.ContractBackend, string) (string, error)
		wantErr   error
	}{
		{
			desc:      "nil resolve function",
			resolveFn: nil,
			wantErr:   ens.ErrNotImplemented,
		},
		{
			desc: "resolve function internal error",
			resolveFn: func(bind.ContractBackend, string) (string, error) {
				return "", errors.New("internal error")
			},
			wantErr: ens.ErrResolveFailed,
		},
		{
			desc: "resolver returns empty string",
			resolveFn: func(bind.ContractBackend, string) (string, error) {
				return "", nil
			},
			wantErr: ens.ErrInvalidContentHash,
		},
		{
			desc: "resolve does not prefix address with /swarm",
			resolveFn: func(bind.ContractBackend, string) (string, error) {
				return addr.String(), nil
			},
			wantErr: ens.ErrInvalidContentHash,
		},
		{
			desc: "resolve returns prefixed address",
			resolveFn: func(bind.ContractBackend, string) (string, error) {
				return ens.SwarmContentHashPrefix + addr.String(), nil
			},
			wantErr: ens.ErrInvalidContentHash,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			cl, err := ens.NewClient("example.com",
				ens.WithDialFunc(func(string) (*ethclient.Client, error) {
					return nil, nil
				}),
				ens.WithResolveFunc(tC.resolveFn),
			)
			if err != nil {
				t.Fatal(err)
			}
			_, err = cl.Resolve(tC.name)
			if err != nil {
				if !errors.Is(err, tC.wantErr) {
					t.Errorf("got %v, want %v", err, tC.wantErr)
				}
				return
			}
		})
	}
}
