// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
)

type chequeSignerMock struct {
	sign func(cheque *chequebook.Cheque) ([]byte, error)
}

func (m *chequeSignerMock) Sign(cheque *chequebook.Cheque) ([]byte, error) {
	return m.sign(cheque)
}

type factoryMock struct {
	erc20Address     func(ctx context.Context) (common.Address, error)
	deploy           func(ctx context.Context, issuer common.Address, defaultHardDepositTimeoutDuration *big.Int) (common.Hash, error)
	waitDeployed     func(ctx context.Context, txHash common.Hash) (common.Address, error)
	verifyBytecode   func(ctx context.Context) error
	verifyChequebook func(ctx context.Context, chequebook common.Address) error
}

// ERC20Address returns the token for which this factory deploys chequebooks.
func (m *factoryMock) ERC20Address(ctx context.Context) (common.Address, error) {
	return m.erc20Address(ctx)
}

func (m *factoryMock) Deploy(ctx context.Context, issuer common.Address, defaultHardDepositTimeoutDuration *big.Int) (common.Hash, error) {
	return m.deploy(ctx, issuer, defaultHardDepositTimeoutDuration)
}

func (m *factoryMock) WaitDeployed(ctx context.Context, txHash common.Hash) (common.Address, error) {
	return m.waitDeployed(ctx, txHash)
}

// VerifyBytecode checks that the factory is valid.
func (m *factoryMock) VerifyBytecode(ctx context.Context) error {
	return m.verifyBytecode(ctx)
}

// VerifyChequebook checks that the supplied chequebook has been deployed by this factory.
func (m *factoryMock) VerifyChequebook(ctx context.Context, chequebook common.Address) error {
	return m.verifyChequebook(ctx, chequebook)
}
