// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ens_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
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
		connectFn    func(string, string) (*ethclient.Client, error)
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
			connectFn: func(s1, s2 string) (*ethclient.Client, error) {
				return nil, errors.New("dial error")
			},
			wantErr: ens.ErrFailedToConnect,
		},
		{
			desc:     "regular endpoint",
			endpoint: "someaddress.org",
			connectFn: func(s1, s2 string) (*ethclient.Client, error) {
				return &ethclient.Client{}, nil
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
			ens.WithConnectFunc(func(endpoint, contractAddr string) (*ethclient.Client, error) {
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
		t.Parallel()

		cl, err := ens.NewClient("",
			ens.WithConnectFunc(func(endpoint, contractAddr string) (*ethclient.Client, error) {
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
	t.Parallel()

	testContractAddrString := "00000000000C2E074eC69A0dFb2997BA6C702e1B"
	testContractAddr := common.HexToAddress(testContractAddrString)
	testSwarmAddr := swarm.MustParseHexAddress("aaabbbcc")

	testCases := []struct {
		desc         string
		name         string
		contractAddr string
		resolveFn    func(*ethclient.Client, common.Address, string) (string, error)
		wantErr      error
	}{
		{
			desc:      "nil resolve function",
			resolveFn: nil,
			wantErr:   ens.ErrNotImplemented,
		},
		{
			desc: "resolve function internal error",
			resolveFn: func(*ethclient.Client, common.Address, string) (string, error) {
				return "", errors.New("internal error")
			},
			wantErr: ens.ErrResolveFailed,
		},
		{
			desc: "resolver returns empty string",
			resolveFn: func(*ethclient.Client, common.Address, string) (string, error) {
				return "", nil
			},
			wantErr: resolver.ErrInvalidContentHash,
		},
		{
			desc: "resolve does not prefix address with /swarm",
			resolveFn: func(*ethclient.Client, common.Address, string) (string, error) {
				return testSwarmAddr.String(), nil
			},
			wantErr: resolver.ErrInvalidContentHash,
		},
		{
			desc: "resolve returns prefixed address",
			resolveFn: func(*ethclient.Client, common.Address, string) (string, error) {
				return ens.SwarmContentHashPrefix + testSwarmAddr.String(), nil
			},
			wantErr: resolver.ErrInvalidContentHash,
		},
		{
			desc: "expect properly set contract address",
			resolveFn: func(b *ethclient.Client, c common.Address, s string) (string, error) {
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
				ens.WithConnectFunc(func(endpoint, contractAddr string) (*ethclient.Client, error) {
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

// revertError is a JSON-RPC error carrying contract revert data as a hex string.
type revertError struct{ data []byte }

func (e revertError) Error() string  { return "execution reverted" }
func (e revertError) ErrorCode() int { return 3 }
func (e revertError) ErrorData() any { return hexutil.Encode(e.data) }

// revertBytesError is a JSON-RPC error carrying raw revert bytes.
type revertBytesError struct{ data []byte }

func (e revertBytesError) Error() string  { return "execution reverted" }
func (e revertBytesError) ErrorCode() int { return 3 }
func (e revertBytesError) ErrorData() any { return e.data }

func callToAddress(v any) (common.Address, bool) {
	switch to := v.(type) {
	case string:
		return common.HexToAddress(to), true
	case *common.Address:
		if to == nil {
			return common.Address{}, false
		}
		return *to, true
	case common.Address:
		return to, true
	default:
		return common.Address{}, false
	}
}

// universalResolverService serves eth_call requests as the ENS Universal
// Resolver would.
type universalResolverService struct {
	t        *testing.T
	wantTo   common.Address
	wantName []byte
	wantData []byte
	output   []byte
	err      error
}

func (s *universalResolverService) Call(_ context.Context, args map[string]any, _ string) (hexutil.Bytes, error) {
	to, ok := callToAddress(args["to"])
	if !ok {
		s.t.Errorf("to: unexpected type %T", args["to"])
	} else if to != s.wantTo {
		s.t.Errorf("to: got %s, want %s", to, s.wantTo)
	}
	hexInput, _ := args["input"].(string)
	input, err := hexutil.Decode(hexInput)
	if err != nil {
		s.t.Errorf("decode input: %v", err)
		return nil, err
	}
	method := ens.UniversalResolverABI.Methods["resolve"]
	if !bytes.Equal(input[:4], method.ID) {
		s.t.Errorf("selector: got %x, want %x", input[:4], method.ID)
	}
	values, err := method.Inputs.Unpack(input[4:])
	if err != nil {
		s.t.Errorf("unpack input: %v", err)
		return nil, err
	}
	if got := values[0].([]byte); !bytes.Equal(got, s.wantName) {
		s.t.Errorf("name: got %x, want %x", got, s.wantName)
	}
	if got := values[1].([]byte); !bytes.Equal(got, s.wantData) {
		s.t.Errorf("data: got %x, want %x", got, s.wantData)
	}
	return s.output, s.err
}

func TestResolveUniversalResolver(t *testing.T) {
	t.Parallel()

	urABI := ens.UniversalResolverABI
	testSwarmAddr := swarm.MustParseHexAddress("7f881a6fb1700c8b10c355690ead159573f3ad8269eb74fc0b32d231798f52c2")
	contractAddr := common.HexToAddress("eEeEEEeE14D718C2B47D9923Deab1335E144EeEe")
	resolverAddr := common.HexToAddress("0x7f3cfC22B509E1b7d88F7234C007846ADC68089D")
	dnsName := []byte("\x0bsolarpunkid\x03eth\x00")

	node, err := goens.NameHash("solarpunkid.eth")
	if err != nil {
		t.Fatal(err)
	}
	wantData, err := urABI.Pack("contenthash", node)
	if err != nil {
		t.Fatal(err)
	}

	resolveOutput := func(t *testing.T, ch []byte) []byte {
		t.Helper()
		inner, err := urABI.Methods["contenthash"].Outputs.Pack(ch)
		if err != nil {
			t.Fatal(err)
		}
		out, err := urABI.Methods["resolve"].Outputs.Pack(inner, resolverAddr)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	revert := func(t *testing.T, name string, args ...any) error {
		t.Helper()
		abiErr := urABI.Errors[name]
		data, err := abiErr.Inputs.Pack(args...)
		if err != nil {
			t.Fatal(err)
		}
		return revertError{data: append(abiErr.ID[:4:4], data...)}
	}

	bzzHash, err := goens.StringToContenthash(ens.SwarmContentHashPrefix + testSwarmAddr.String())
	if err != nil {
		t.Fatal(err)
	}

	okOutput := resolveOutput(t, bzzHash)

	testCases := []struct {
		desc     string
		name     string
		output   []byte
		err      error
		wantAddr swarm.Address
		wantErr  error
	}{
		{
			desc:     "ok",
			name:     "solarpunkid.eth",
			output:   okOutput,
			wantAddr: testSwarmAddr,
		},
		{
			desc:     "name is normalized",
			name:     "SolarPunkID.eth",
			output:   okOutput,
			wantAddr: testSwarmAddr,
		},
		{
			desc:    "empty contenthash",
			name:    "solarpunkid.eth",
			output:  resolveOutput(t, nil),
			wantErr: resolver.ErrInvalidContentHash,
		},
		{
			desc:    "resolver not found",
			name:    "solarpunkid.eth",
			err:     revert(t, "ResolverNotFound", dnsName),
			wantErr: resolver.ErrNotFound,
		},
		{
			desc:    "resolver not contract",
			name:    "solarpunkid.eth",
			err:     revert(t, "ResolverNotContract", dnsName, resolverAddr),
			wantErr: resolver.ErrNotFound,
		},
		{
			desc:    "offchain lookup",
			name:    "solarpunkid.eth",
			err:     revert(t, "OffchainLookup", resolverAddr, []string{"https://example.com"}, []byte{}, [4]byte{}, []byte{}),
			wantErr: resolver.ErrServiceNotAvailable,
		},
		{
			desc:    "unsupported resolver profile",
			name:    "solarpunkid.eth",
			err:     revert(t, "UnsupportedResolverProfile", [4]byte{}),
			wantErr: resolver.ErrInvalidContentHash,
		},
		{
			desc:    "resolver error",
			name:    "solarpunkid.eth",
			err:     revert(t, "ResolverError", []byte{0x01}),
			wantErr: resolver.ErrInvalidContentHash,
		},
		{
			desc:    "garbage resolve output",
			name:    "solarpunkid.eth",
			output:  []byte{0x00},
			wantErr: resolver.ErrInvalidContentHash,
		},
		{
			desc:    "unknown revert",
			name:    "solarpunkid.eth",
			err:     revertError{},
			wantErr: resolver.ErrServiceNotAvailable,
		},
		{
			desc:    "endpoint error",
			name:    "solarpunkid.eth",
			err:     errors.New("rate limit exceeded"),
			wantErr: resolver.ErrServiceNotAvailable,
		},
		{
			desc:    "invalid name",
			name:    "solarpunkid..eth",
			wantErr: resolver.ErrNotFound,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			t.Parallel()

			svc := &universalResolverService{
				t:        t,
				wantTo:   contractAddr,
				wantName: dnsName,
				wantData: wantData,
				output:   tC.output,
				err:      tC.err,
			}
			rpcServer := rpc.NewServer()
			t.Cleanup(rpcServer.Stop)
			if err := rpcServer.RegisterName("eth", svc); err != nil {
				t.Fatal(err)
			}
			ethCl := ethclient.NewClient(rpc.DialInProc(rpcServer))

			cl, err := ens.NewClient("",
				ens.WithConnectFunc(func(string, string) (*ethclient.Client, error) {
					return ethCl, nil
				}),
			)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cl.Close() })

			addr, err := cl.Resolve(tC.name)
			if !errors.Is(err, tC.wantErr) {
				t.Fatalf("got error %v, want %v", err, tC.wantErr)
			}
			if !addr.Equal(tC.wantAddr) {
				t.Errorf("got address %s, want %s", addr, tC.wantAddr)
			}
		})
	}
}

func TestResolveErrorBytesData(t *testing.T) {
	t.Parallel()

	abiErr := ens.UniversalResolverABI.Errors["ResolverNotFound"]
	data, err := abiErr.Inputs.Pack([]byte("\x0bsolarpunkid\x03eth\x00"))
	if err != nil {
		t.Fatal(err)
	}

	got := ens.ResolveError(revertBytesError{data: append(abiErr.ID[:4:4], data...)})
	if !errors.Is(got, resolver.ErrNotFound) {
		t.Fatalf("got error %v, want %v", got, resolver.ErrNotFound)
	}
}

func TestDNSEncode(t *testing.T) {
	t.Parallel()

	for _, tC := range []struct {
		name    string
		want    []byte
		wantErr bool
	}{
		{name: "eth", want: []byte("\x03eth\x00")},
		{name: "swarm.eth", want: []byte("\x05swarm\x03eth\x00")},
		{name: strings.Repeat("a", 253), want: append(append([]byte{253}, strings.Repeat("a", 253)...), 0)},
		{name: "", wantErr: true},
		{name: ".eth", wantErr: true},
		{name: "swarm.", wantErr: true},
		{name: "swarm..eth", wantErr: true},
		{name: strings.Repeat("a", 254), wantErr: true},
		{name: strings.Repeat("a", 255), wantErr: true},
		{name: strings.Repeat("a", 256), wantErr: true},
	} {
		got, err := ens.DNSEncode(tC.name)
		if (err != nil) != tC.wantErr {
			t.Fatalf("%q: got error %v, want error %v", tC.name, err, tC.wantErr)
		}
		if !bytes.Equal(got, tC.want) {
			t.Errorf("%q: got %x, want %x", tC.name, got, tC.want)
		}
	}
}
