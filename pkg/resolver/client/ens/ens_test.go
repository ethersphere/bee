// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ens_test

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	goens "github.com/wealdtech/go-ens/v3"

	"github.com/ethersphere/bee/v2/pkg/resolver"
	"github.com/ethersphere/bee/v2/pkg/resolver/client/ens"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestNewENSClient(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

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
	t.Parallel()

	t.Run("connected", func(t *testing.T) {
		t.Parallel()

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
		t.Parallel()

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
	t.Parallel()

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
			wantErr: resolver.ErrInvalidContentHash,
		},
		{
			desc: "resolve does not prefix address with /swarm",
			resolveFn: func(*goens.Registry, common.Address, string) (string, error) {
				return testSwarmAddr.String(), nil
			},
			wantErr: resolver.ErrInvalidContentHash,
		},
		{
			desc: "resolve returns prefixed address",
			resolveFn: func(*goens.Registry, common.Address, string) (string, error) {
				return ens.SwarmContentHashPrefix + testSwarmAddr.String(), nil
			},
			wantErr: resolver.ErrInvalidContentHash,
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
			t.Parallel()

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

// TestResolveDirect covers the path taken when the configured contract is a
// resolver (it serves the ENS resolver profile itself) rather than a registry:
// the dial function returns a live client and no registry.
func TestResolveDirect(t *testing.T) {
	t.Parallel()

	testContractAddrString := "9D51D507BC7264d4fE8Ad1cf7Fe191933A0a81d6"
	testContractAddr := common.HexToAddress(testContractAddrString)
	testSwarmAddr := swarm.MustParseHexAddress("aaabbbcc")

	testCases := []struct {
		desc            string
		name            string
		resolveDirectFn func(*ethclient.Client, common.Address, string) (string, error)
		wantErr         error
		wantAddr        swarm.Address
	}{
		{
			desc:            "nil direct resolve function",
			resolveDirectFn: nil,
			wantErr:         ens.ErrNotImplemented,
		},
		{
			desc: "not registered",
			resolveDirectFn: func(*ethclient.Client, common.Address, string) (string, error) {
				return "", resolver.ErrNotFound
			},
			wantErr: resolver.ErrNotFound,
		},
		{
			desc: "resolves against the configured contract",
			name: "example.gwei",
			resolveDirectFn: func(_ *ethclient.Client, c common.Address, name string) (string, error) {
				if c != testContractAddr {
					return "", errors.New("invalid contract address")
				}
				if name != "example.gwei" {
					return "", errors.New("invalid name")
				}
				return ens.SwarmContentHashPrefix + testSwarmAddr.String(), nil
			},
			wantAddr: testSwarmAddr,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			t.Parallel()

			cl, err := ens.NewClient("example.com",
				ens.WithContractAddress(testContractAddrString),
				ens.WithConnectFunc(func(endpoint, contractAddr string) (*ethclient.Client, *goens.Registry, error) {
					return &ethclient.Client{}, nil, nil // connected, no registry: a resolver contract
				}),
				ens.WithResolveFunc(func(*goens.Registry, common.Address, string) (string, error) {
					return "", errors.New("registry path must not be used for a resolver contract")
				}),
				ens.WithResolveDirectFunc(tC.resolveDirectFn),
			)
			if err != nil {
				t.Fatal(err)
			}
			got, err := cl.Resolve(tC.name)
			if err != nil {
				if !errors.Is(err, tC.wantErr) {
					t.Errorf("got %v, want %v", err, tC.wantErr)
				}
				return
			}
			if tC.wantErr != nil {
				t.Fatalf("got no error, want %v", tC.wantErr)
			}
			if !got.Equal(tC.wantAddr) {
				t.Errorf("got %s, want %s", got, tC.wantAddr)
			}
		})
	}
}

type fakeCaller struct {
	out     []byte
	err     error
	gotCall ethereum.CallMsg
}

func (f *fakeCaller) CallContract(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	f.gotCall = call
	return f.out, f.err
}

func TestSupportsContenthash(t *testing.T) {
	t.Parallel()

	addr := common.HexToAddress("9D51D507BC7264d4fE8Ad1cf7Fe191933A0a81d6")
	word := func(last byte) []byte { b := make([]byte, 32); b[31] = last; return b }

	testCases := []struct {
		desc    string
		out     []byte
		err     error
		want    bool
		wantErr bool
	}{
		{desc: "supports the contenthash profile", out: word(1), want: true},
		{desc: "does not support it", out: word(0), want: false},
		{desc: "no EIP-165 (empty return)", out: nil, want: false},
		{desc: "call error", err: errors.New("rpc down"), wantErr: true},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			t.Parallel()

			fc := &fakeCaller{out: tC.out, err: tC.err}
			got, err := ens.SupportsContenthash(context.Background(), fc, addr)
			if (err != nil) != tC.wantErr {
				t.Fatalf("error: got %v, wantErr %v", err, tC.wantErr)
			}
			if got != tC.want {
				t.Errorf("got %v, want %v", got, tC.want)
			}
			if fc.gotCall.To == nil || *fc.gotCall.To != addr {
				t.Errorf("call target: got %v, want %v", fc.gotCall.To, addr)
			}
			// supportsInterface(bytes4) selector followed by the contenthash
			// interface id in a 32-byte word.
			wantData := append([]byte{0x01, 0xff, 0xc9, 0xa7, 0xbc, 0x1c, 0x58, 0xd1}, make([]byte, 28)...)
			if string(fc.gotCall.Data) != string(wantData) {
				t.Errorf("call data: got %x, want %x", fc.gotCall.Data, wantData)
			}
		})
	}
}
