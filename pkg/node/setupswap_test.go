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
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/node"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/erc20"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/transaction"
)

// These tests pin down behavior of the SwapEnable / ChequebookEnable /
// chainEnabled block that used to be inlined in NewBee. They give future
// refactors of that block — e.g. issue #5233, which changes the SwapEnable=false
// path — a regression net for every behavior that is *not* supposed to change.

// recordedSwapDeps wraps a node.SwapDeps so each test controls success / failure
// of every injected call and observes which ones ran.
type recordedSwapDeps struct {
	factory      chequebook.Factory
	factoryErr   error
	factoryCalls int

	svc      chequebook.Service
	svcErr   error
	svcCalls int

	store        chequebook.ChequeStore
	cashout      chequebook.CashoutService
	cashoutCalls int
}

func (r *recordedSwapDeps) deps() node.SwapDeps {
	return node.SwapDeps{
		InitFactory: func(_ log.Logger, _ transaction.Backend, _ int64, _ transaction.Service, _ string) (chequebook.Factory, error) {
			r.factoryCalls++
			if r.factoryErr != nil {
				return nil, r.factoryErr
			}
			return r.factory, nil
		},
		InitChequebookService: func(_ context.Context, _ log.Logger, _ storage.StateStorer, _ crypto.Signer, _ int64, _ transaction.Backend, _ common.Address, _ transaction.Service, _ chequebook.Factory, _ string, _ erc20.Service) (chequebook.Service, error) {
			r.svcCalls++
			if r.svcErr != nil {
				return nil, r.svcErr
			}
			return r.svc, nil
		},
		InitChequeStoreCashout: func(_ storage.StateStorer, _ transaction.Backend, _ chequebook.Factory, _ int64, _ common.Address, _ transaction.Service) (chequebook.ChequeStore, chequebook.CashoutService) {
			r.cashoutCalls++
			return r.store, r.cashout
		},
	}
}

// callSetupSwap is a convenience wrapper that injects a real-looking transaction
// service (nil here, since the fakes never touch it) and runs setupSwap.
func callSetupSwap(t *testing.T, o *node.Options, chainEnabled bool, deps node.SwapDeps) (node.SwapResult, error) {
	t.Helper()
	return node.SetupSwap(
		context.Background(),
		log.Noop,
		o,
		chainEnabled,
		nil, // chainBackend — fakes never invoke it
		0,   // chainID
		nil, // transactionService — fakes never invoke it
		nil, // stateStore
		nil, // signer
		common.Address{},
		deps,
	)
}

func TestSetupSwap_DisabledMakesNoCallsAndReturnsZero(t *testing.T) {
	t.Parallel()

	r := &recordedSwapDeps{factory: &swapFactoryStub{}}
	res, err := callSetupSwap(t,
		&node.Options{SwapEnable: false},
		true, // chainEnabled is irrelevant when SwapEnable=false
		r.deps(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Erc20Service != nil || res.ChequebookService != nil || res.ChequeStore != nil || res.CashoutService != nil {
		t.Fatalf("expected zero swapResult, got %+v", res)
	}
	if r.factoryCalls+r.svcCalls+r.cashoutCalls != 0 {
		t.Fatalf("expected no deps to be invoked, got factory=%d svc=%d cashout=%d",
			r.factoryCalls, r.svcCalls, r.cashoutCalls)
	}
}

func TestSetupSwap_FactoryErrorReturnsWrapped(t *testing.T) {
	t.Parallel()

	want := errors.New("boom")
	r := &recordedSwapDeps{factoryErr: want}
	res, err := callSetupSwap(t,
		&node.Options{SwapEnable: true, ChequebookEnable: true},
		true,
		r.deps(),
	)
	if !errors.Is(err, want) {
		t.Fatalf("expected wrapped %v, got %v", want, err)
	}
	if !strings.Contains(err.Error(), "init chequebook factory") {
		t.Fatalf("expected error to start with %q, got %q", "init chequebook factory", err.Error())
	}
	if res.Erc20Service != nil || res.ChequeStore != nil || res.CashoutService != nil {
		t.Fatal("no downstream output should be set when factory init fails")
	}
	if r.svcCalls != 0 || r.cashoutCalls != 0 {
		t.Fatal("downstream deps must not be invoked after factory error")
	}
}

func TestSetupSwap_ERC20AddressErrorReturnsWrapped(t *testing.T) {
	t.Parallel()

	want := errors.New("rpc dead")
	r := &recordedSwapDeps{factory: &swapFactoryStub{erc20Err: want}}
	res, err := callSetupSwap(t,
		&node.Options{SwapEnable: true, ChequebookEnable: true},
		true,
		r.deps(),
	)
	if !errors.Is(err, want) {
		t.Fatalf("expected wrapped %v, got %v", want, err)
	}
	if !strings.Contains(err.Error(), "factory fail") {
		t.Fatalf("expected error to start with %q, got %q", "factory fail", err.Error())
	}
	if r.svcCalls != 0 || r.cashoutCalls != 0 {
		t.Fatal("downstream deps must not be invoked after ERC20 error")
	}
	if res.Erc20Service != nil {
		t.Fatal("erc20Service must not be set when ERC20Address fails")
	}
}

func TestSetupSwap_ChequebookDisabled_StillSetsErc20AndCashout(t *testing.T) {
	t.Parallel()

	wantStore := &chequeStoreStub{}
	wantCashout := &cashoutStub{}
	r := &recordedSwapDeps{factory: &swapFactoryStub{}, store: wantStore, cashout: wantCashout}
	res, err := callSetupSwap(t,
		&node.Options{SwapEnable: true, ChequebookEnable: false},
		true,
		r.deps(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.svcCalls != 0 {
		t.Fatalf("chequebook service must not be initialized when ChequebookEnable=false, got %d calls", r.svcCalls)
	}
	if res.ChequebookService != nil {
		t.Fatal("chequebookService must remain nil when ChequebookEnable=false")
	}
	if res.Erc20Service == nil {
		t.Fatal("erc20Service must be set when SwapEnable=true")
	}
	if res.ChequeStore != wantStore || res.CashoutService != wantCashout {
		t.Fatal("chequeStore / cashoutService must be set when SwapEnable=true")
	}
}

func TestSetupSwap_ChainDisabled_SkipsChequebookServiceOnly(t *testing.T) {
	t.Parallel()

	wantStore := &chequeStoreStub{}
	wantCashout := &cashoutStub{}
	r := &recordedSwapDeps{factory: &swapFactoryStub{}, store: wantStore, cashout: wantCashout}
	res, err := callSetupSwap(t,
		&node.Options{SwapEnable: true, ChequebookEnable: true},
		false, // chainEnabled=false gates the chequebook service even when ChequebookEnable=true
		r.deps(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.svcCalls != 0 {
		t.Fatalf("chequebook service must not be initialized when chainEnabled=false, got %d calls", r.svcCalls)
	}
	if res.ChequebookService != nil {
		t.Fatal("chequebookService must remain nil when chainEnabled=false")
	}
	if res.Erc20Service == nil || res.ChequeStore != wantStore || res.CashoutService != wantCashout {
		t.Fatal("erc20Service / chequeStore / cashoutService must still be set")
	}
}

func TestSetupSwap_ChequebookServiceError_ReturnsWrapped(t *testing.T) {
	t.Parallel()

	want := errors.New("svc no")
	r := &recordedSwapDeps{factory: &swapFactoryStub{}, svcErr: want}
	_, err := callSetupSwap(t,
		&node.Options{SwapEnable: true, ChequebookEnable: true},
		true,
		r.deps(),
	)
	if !errors.Is(err, want) {
		t.Fatalf("expected wrapped %v, got %v", want, err)
	}
	if !strings.Contains(err.Error(), "init chequebook service") {
		t.Fatalf("expected error to contain %q, got %q", "init chequebook service", err.Error())
	}
	// The original block returned before initChequeStoreCashout when chequebook
	// service init failed; verify that contract is preserved.
	if r.cashoutCalls != 0 {
		t.Fatal("initChequeStoreCashout must not run after chequebook service error")
	}
}

func TestSetupSwap_AllEnabledSuccess_SetsEveryOutput(t *testing.T) {
	t.Parallel()

	wantSvc := &chequebookSvcStub{}
	wantStore := &chequeStoreStub{}
	wantCashout := &cashoutStub{}
	r := &recordedSwapDeps{
		factory: &swapFactoryStub{},
		svc:     wantSvc,
		store:   wantStore,
		cashout: wantCashout,
	}
	res, err := callSetupSwap(t,
		&node.Options{SwapEnable: true, ChequebookEnable: true},
		true,
		r.deps(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Erc20Service == nil {
		t.Fatal("erc20Service not set")
	}
	if res.ChequebookService != wantSvc {
		t.Fatal("chequebookService not set to the value returned by deps.InitChequebookService")
	}
	if res.ChequeStore != wantStore || res.CashoutService != wantCashout {
		t.Fatal("chequeStore / cashoutService not set to the values returned by deps.InitChequeStoreCashout")
	}
	if r.factoryCalls != 1 || r.svcCalls != 1 || r.cashoutCalls != 1 {
		t.Fatalf("each dep should be called exactly once, got factory=%d svc=%d cashout=%d",
			r.factoryCalls, r.svcCalls, r.cashoutCalls)
	}
}

// ---- minimal stubs for chequebook interfaces used in these tests ----

type swapFactoryStub struct{ erc20Err error }

func (f *swapFactoryStub) ERC20Address(_ context.Context) (common.Address, error) {
	if f.erc20Err != nil {
		return common.Address{}, f.erc20Err
	}
	return common.Address{}, nil
}

func (f *swapFactoryStub) Deploy(_ context.Context, _ common.Address, _ *big.Int, _ common.Hash) (common.Hash, error) {
	panic("Deploy must not be called by setupSwap")
}

func (f *swapFactoryStub) WaitDeployed(_ context.Context, _ common.Hash) (common.Address, error) {
	panic("WaitDeployed must not be called by setupSwap")
}

func (f *swapFactoryStub) VerifyChequebook(_ context.Context, _ common.Address) error {
	panic("VerifyChequebook must not be called by setupSwap")
}

func (f *swapFactoryStub) VerifyBytecode(_ context.Context) error {
	panic("VerifyBytecode must not be called by setupSwap")
}

type chequebookSvcStub struct{}

func (chequebookSvcStub) Deposit(context.Context, *big.Int) (common.Hash, error) {
	panic("Deposit must not be called")
}

func (chequebookSvcStub) Withdraw(context.Context, *big.Int) (common.Hash, error) {
	panic("Withdraw must not be called")
}

func (chequebookSvcStub) WaitForDeposit(context.Context, common.Hash) error {
	panic("WaitForDeposit must not be called")
}

func (chequebookSvcStub) Balance(context.Context) (*big.Int, error) {
	panic("Balance must not be called")
}

func (chequebookSvcStub) AvailableBalance(context.Context) (*big.Int, error) {
	panic("AvailableBalance must not be called")
}

func (chequebookSvcStub) Address() common.Address {
	panic("Address must not be called")
}

func (chequebookSvcStub) Issue(context.Context, common.Address, *big.Int, chequebook.SendChequeFunc) (*big.Int, error) {
	panic("Issue must not be called")
}

func (chequebookSvcStub) LastCheque(common.Address) (*chequebook.SignedCheque, error) {
	panic("LastCheque must not be called")
}

func (chequebookSvcStub) LastCheques() (map[common.Address]*chequebook.SignedCheque, error) {
	panic("LastCheques must not be called")
}

type chequeStoreStub struct{}

func (chequeStoreStub) ReceiveCheque(context.Context, *chequebook.SignedCheque, *big.Int, *big.Int) (*big.Int, error) {
	panic("ReceiveCheque must not be called")
}

func (chequeStoreStub) LastCheque(common.Address) (*chequebook.SignedCheque, error) {
	panic("LastCheque must not be called")
}

func (chequeStoreStub) LastCheques() (map[common.Address]*chequebook.SignedCheque, error) {
	panic("LastCheques must not be called")
}

type cashoutStub struct{}

func (cashoutStub) CashCheque(context.Context, common.Address, common.Address) (common.Hash, error) {
	panic("CashCheque must not be called")
}

func (cashoutStub) CashoutStatus(context.Context, common.Address) (*chequebook.CashoutStatus, error) {
	panic("CashoutStatus must not be called")
}
