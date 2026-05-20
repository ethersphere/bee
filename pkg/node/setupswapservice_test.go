// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/node"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/v2/pkg/settlement"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/priceoracle"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/transaction"
)

// These tests cover setupSwapService — the second swap block in NewBee, which
// is gated on (SwapEnable && chainEnabled) and inside that gate wires up the
// accounting PayFunc only when ChequebookEnable is true.

type recordedSwapServiceDeps struct {
	calls       int
	resultSwap  *swap.Service
	resultPrice priceoracle.Service
	resultErr   error
}

func (r *recordedSwapServiceDeps) deps() node.SwapServiceDeps {
	return node.SwapServiceDeps{
		InitSwap: func(_ *libp2p.Service, _ log.Logger, _ storage.StateStorer, _ uint64, _ common.Address, _ chequebook.Service, _ chequebook.ChequeStore, _ chequebook.CashoutService, _ settlement.Accounting, _ string, _ int64, _ transaction.Service) (*swap.Service, priceoracle.Service, error) {
			r.calls++
			if r.resultErr != nil {
				return nil, nil, r.resultErr
			}
			return r.resultSwap, r.resultPrice, nil
		},
	}
}

// realSwapService builds an actual *swap.Service so that swapService.Pay
// resolves to a callable method value. Most fields are nil — the tests never
// invoke Pay, they just check that PayFunc was assigned when expected.
func realSwapService(t *testing.T) *swap.Service {
	t.Helper()
	return swap.New(nil, log.Noop, nil, nil, nil, nil, 0, nil, nil, common.Address{})
}

func callSetupSwapService(t *testing.T, o *node.Options, chainEnabled bool, deps node.SwapServiceDeps) (node.SwapServiceResult, error) {
	t.Helper()
	return node.SetupSwapService(
		o, chainEnabled,
		nil, log.Noop, nil, 0, common.Address{},
		nil, nil, nil, nil,
		0, nil,
		deps,
	)
}

func TestSetupSwapService_SwapDisabled_MakesNoCalls(t *testing.T) {
	t.Parallel()

	r := &recordedSwapServiceDeps{}
	res, err := callSetupSwapService(t, &node.Options{SwapEnable: false, ChequebookEnable: true}, true, r.deps())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.calls != 0 {
		t.Fatalf("InitSwap must not run when SwapEnable=false, got %d calls", r.calls)
	}
	if res.SwapService != nil || res.PriceOracle != nil || res.PayFunc != nil {
		t.Fatalf("expected zero result, got %+v", res)
	}
}

func TestSetupSwapService_ChainDisabled_MakesNoCalls(t *testing.T) {
	t.Parallel()

	r := &recordedSwapServiceDeps{}
	res, err := callSetupSwapService(t, &node.Options{SwapEnable: true, ChequebookEnable: true}, false, r.deps())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.calls != 0 {
		t.Fatalf("InitSwap must not run when chainEnabled=false, got %d calls", r.calls)
	}
	if res.SwapService != nil || res.PriceOracle != nil || res.PayFunc != nil {
		t.Fatalf("expected zero result, got %+v", res)
	}
}

func TestSetupSwapService_InitSwapError_Wraps(t *testing.T) {
	t.Parallel()

	want := errors.New("init swap exploded")
	r := &recordedSwapServiceDeps{resultErr: want}
	res, err := callSetupSwapService(t, &node.Options{SwapEnable: true, ChequebookEnable: true}, true, r.deps())
	if !errors.Is(err, want) {
		t.Fatalf("expected wrapped %v, got %v", want, err)
	}
	if !strings.Contains(err.Error(), "init swap service") {
		t.Fatalf("expected error to start with %q, got %q", "init swap service", err.Error())
	}
	if res.SwapService != nil || res.PriceOracle != nil || res.PayFunc != nil {
		t.Fatalf("expected zero result on error, got %+v", res)
	}
}

func TestSetupSwapService_Enabled_ChequebookDisabled_NoPayFunc(t *testing.T) {
	t.Parallel()

	wantSwap := realSwapService(t)
	wantPrice := &priceOracleStub{}
	r := &recordedSwapServiceDeps{resultSwap: wantSwap, resultPrice: wantPrice}

	res, err := callSetupSwapService(t, &node.Options{SwapEnable: true, ChequebookEnable: false}, true, r.deps())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.calls != 1 {
		t.Fatalf("InitSwap should run exactly once, got %d", r.calls)
	}
	if res.SwapService != wantSwap {
		t.Fatal("SwapService not propagated")
	}
	if res.PriceOracle != wantPrice {
		t.Fatal("PriceOracle not propagated")
	}
	if res.PayFunc != nil {
		t.Fatal("PayFunc must be nil when ChequebookEnable=false")
	}
}

func TestSetupSwapService_Enabled_ChequebookEnabled_SetsPayFunc(t *testing.T) {
	t.Parallel()

	wantSwap := realSwapService(t)
	wantPrice := &priceOracleStub{}
	r := &recordedSwapServiceDeps{resultSwap: wantSwap, resultPrice: wantPrice}

	res, err := callSetupSwapService(t, &node.Options{SwapEnable: true, ChequebookEnable: true}, true, r.deps())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.SwapService != wantSwap || res.PriceOracle != wantPrice {
		t.Fatal("Swap/PriceOracle not propagated")
	}
	if res.PayFunc == nil {
		t.Fatal("PayFunc must be set when SwapEnable && chainEnabled && ChequebookEnable")
	}
}

// priceOracleStub is a no-op priceoracle.Service so we have an identity-comparable
// value to thread through the result struct.
type priceOracleStub struct{}

func (priceOracleStub) Start()       {}
func (priceOracleStub) Close() error { return nil }
func (priceOracleStub) GetPrice(context.Context) (*big.Int, *big.Int, error) {
	return nil, nil, nil
}
func (priceOracleStub) CurrentRates() (*big.Int, *big.Int, error) { return nil, nil, nil }
