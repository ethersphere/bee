// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
)

func Init(
	ctx context.Context,
	chequebookFactory Factory,
	stateStore storage.StateStorer,
	logger logging.Logger,
	swapInitialDeposit uint64,
	transactionService TransactionService,
	swapBackend Backend,
	overlayEthAddress common.Address) (chequebookService Service, err error) {
	err = chequebookFactory.VerifyBytecode(ctx)
	if err != nil {
		return nil, err
	}

	erc20Address, err := chequebookFactory.ERC20Address(ctx)
	if err != nil {
		return nil, err
	}

	var chequebookAddress common.Address
	err = stateStore.Get("chequebook", &chequebookAddress)
	if err != nil {
		if err != storage.ErrNotFound {
			return nil, err
		}
		logger.Info("deploying new chequebook")

		chequebookAddress, err = chequebookFactory.Deploy(ctx, overlayEthAddress, big.NewInt(0))
		if err != nil {
			return nil, err
		}

		logger.Infof("deployed to address %x", chequebookAddress)

		err = stateStore.Put("chequebook", chequebookAddress)
		if err != nil {
			return nil, err
		}

		chequebookService, err = New(swapBackend, transactionService, chequebookAddress, erc20Address, overlayEthAddress, NewSimpleSwapBindings, NewERC20Bindings)
		if err != nil {
			return nil, err
		}

		if swapInitialDeposit != 0 {
			logger.Info("depositing into new chequebook")

			depositHash, err := chequebookService.Deposit(ctx, big.NewInt(int64(swapInitialDeposit)))
			if err != nil {
				return nil, err
			}

			err = chequebookService.WaitForDeposit(ctx, depositHash)
			if err != nil {
				return nil, err
			}

			logger.Infof("deposited to chequebook %x in transaction %x", chequebookAddress, depositHash)
		}
	} else {
		chequebookService, err = New(swapBackend, transactionService, chequebookAddress, erc20Address, overlayEthAddress, NewSimpleSwapBindings, NewERC20Bindings)
		if err != nil {
			return nil, err
		}

		logger.Infof("using existing chequebook %x", chequebookAddress)
	}

	err = chequebookFactory.VerifyChequebook(ctx, chequebookService.Address())
	if err != nil {
		return nil, err
	}

	return chequebookService, nil
}
