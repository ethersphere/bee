// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
)

const chequebookKey = "chequebook"

// Init initialises the chequebook service.
func Init(
	ctx context.Context,
	chequebookFactory Factory,
	stateStore storage.StateStorer,
	logger logging.Logger,
	swapInitialDeposit uint64,
	transactionService TransactionService,
	swapBackend Backend,
	overlayEthAddress common.Address,
	chequeSigner ChequeSigner,
	simpleSwapBindingFunc SimpleSwapBindingFunc,
	erc20BindingFunc ERC20BindingFunc) (chequebookService Service, err error) {
	// verify that the supplied factory is valid
	err = chequebookFactory.VerifyBytecode(ctx)
	if err != nil {
		return nil, err
	}

	erc20Address, err := chequebookFactory.ERC20Address(ctx)
	if err != nil {
		return nil, err
	}

	var chequebookAddress common.Address
	err = stateStore.Get(chequebookKey, &chequebookAddress)
	if err != nil {
		if err != storage.ErrNotFound {
			return nil, err
		}

		logger.Info("no chequebook found, deploying new one.")
		if swapInitialDeposit != 0 {
			erc20Token, err := erc20BindingFunc(erc20Address, swapBackend)
			if err != nil {
				return nil, err
			}

			balance, err := erc20Token.BalanceOf(&bind.CallOpts{
				Context: ctx,
			}, overlayEthAddress)
			if err != nil {
				return nil, err
			}

			if balance.Cmp(big.NewInt(int64(swapInitialDeposit))) < 0 {
				return nil, fmt.Errorf("insufficient token for initial deposit. Please make sure there is sufficient eth and bzz available on %x", overlayEthAddress)
			}
		}

		// if we don't yet have a chequebook, deploy a new one
		txHash, err := chequebookFactory.Deploy(ctx, overlayEthAddress, big.NewInt(0))
		if err != nil {
			return nil, err
		}

		logger.Infof("deploying new chequebook in transaction %x", txHash)

		chequebookAddress, err = chequebookFactory.WaitDeployed(ctx, txHash)
		if err != nil {
			return nil, err
		}

		logger.Infof("deployed chequebook at address %x", chequebookAddress)

		// save the address for later use
		err = stateStore.Put(chequebookKey, chequebookAddress)
		if err != nil {
			return nil, err
		}

		chequebookService, err = New(swapBackend, transactionService, chequebookAddress, erc20Address, overlayEthAddress, stateStore, chequeSigner, simpleSwapBindingFunc, erc20BindingFunc)
		if err != nil {
			return nil, err
		}

		if swapInitialDeposit != 0 {
			logger.Infof("depositing %d token into new chequebook", swapInitialDeposit)
			depositHash, err := chequebookService.Deposit(ctx, big.NewInt(int64(swapInitialDeposit)))
			if err != nil {
				return nil, err
			}

			logger.Infof("sent deposit transaction %x", depositHash)
			err = chequebookService.WaitForDeposit(ctx, depositHash)
			if err != nil {
				return nil, err
			}

			logger.Info("successfully deposited to chequebook")
		}
	} else {
		chequebookService, err = New(swapBackend, transactionService, chequebookAddress, erc20Address, overlayEthAddress, stateStore, chequeSigner, simpleSwapBindingFunc, erc20BindingFunc)
		if err != nil {
			return nil, err
		}

		logger.Infof("using existing chequebook %x", chequebookAddress)
	}

	// regardless of how the chequebook service was initialised make sure that the chequebook is valid
	err = chequebookFactory.VerifyChequebook(ctx, chequebookService.Address())
	if err != nil {
		return nil, err
	}

	return chequebookService, nil
}
