// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ens_test

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	goens "github.com/wealdtech/go-ens/v3"

	"github.com/ethersphere/bee/pkg/resolver/client/ens"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestNewENSClient(t *testing.T) {
	testCases := []struct {
		desc         string
		endpoint     string
		address      string
		connectFn    func(string, string) (*ethclient.Client, *goens.Registry, error)
		wantErr      error
		wantEndpoint string
	}{
		{
			desc:      "nil dial function",
			endpoint:  "someaddress.net",
			connectFn: nil,
			wantErr:   ens.ErrNotImplemented,
		},
		{
			desc:     "error in dial function",
			endpoint: "someaddress.com",
			connectFn: func(s1, s2 string) (*ethclient.Client, *goens.Registry, error) {
				return nil, nil, errors.New("dial error")
			},
			wantErr: ens.ErrFailedToConnect,
		},
		{
			desc:     "regular endpoint",
			endpoint: "someaddress.org",
			connectFn: func(s1, s2 string) (*ethclient.Client, *goens.Registry, error) {
				return &ethclient.Client{}, nil, nil
			},
			wantEndpoint: "someaddress.org",
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			cl, err := ens.NewClient(tC.endpoint,
				ens.WithConnectFunc(tC.connectFn),
				ens.WithContractAddress(tC.address),
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
			ens.WithConnectFunc(func(endpoint, contractAddr string) (*ethclient.Client, *goens.Registry, error) {
				return ethCl, nil, nil
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
			ens.WithConnectFunc(func(endpoint, contractAddr string) (*ethclient.Client, *goens.Registry, error) {
				return nil, nil, nil
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
	testContractAddrString := "00000000000C2E074eC69A0dFb2997BA6C702e1B"
	testContractAddr := common.HexToAddress(testContractAddrString)
	testSwarmAddr := swarm.MustParseHexAddress("aaabbbcc")

	testCases := []struct {
		desc         string
		name         string
		contractAddr string
		resolveFn    func(*goens.Registry, common.Address, string) (string, error)
		wantErr      error
	}{
		{
			desc:      "nil resolve function",
			resolveFn: nil,
			wantErr:   ens.ErrNotImplemented,
		},
		{
			desc: "resolve function internal error",
			resolveFn: func(*goens.Registry, common.Address, string) (string, error) {
				return "", errors.New("internal error")
			},
			wantErr: ens.ErrResolveFailed,
		},
		{
			desc: "resolver returns empty string",
			resolveFn: func(*goens.Registry, common.Address, string) (string, error) {
				return "", nil
			},
			wantErr: ens.ErrInvalidContentHash,
		},
		{
			desc: "resolve does not prefix address with /swarm",
			resolveFn: func(*goens.Registry, common.Address, string) (string, error) {
				return testSwarmAddr.String(), nil
			},
			wantErr: ens.ErrInvalidContentHash,
		},
		{
			desc: "resolve returns prefixed address",
			resolveFn: func(*goens.Registry, common.Address, string) (string, error) {
				return ens.SwarmContentHashPrefix + testSwarmAddr.String(), nil
			},
			wantErr: ens.ErrInvalidContentHash,
		},
		{
			desc: "expect properly set contract address",
			resolveFn: func(b *goens.Registry, c common.Address, s string) (string, error) {
				if c != testContractAddr {
					return "", errors.New("invalid contract address")
				}
				return ens.SwarmContentHashPrefix + testSwarmAddr.String(), nil
			},
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			cl, err := ens.NewClient("example.com",
				ens.WithContractAddress(testContractAddrString),
				ens.WithConnectFunc(func(endpoint, contractAddr string) (*ethclient.Client, *goens.Registry, error) {
					return nil, nil, nil
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
