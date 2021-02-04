// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction"
	"github.com/ethersphere/bee/pkg/storage"
)

const chequebookKey = "swap_chequebook"

func checkBalance(
	ctx context.Context,
	logger logging.Logger,
	swapInitialDeposit *big.Int,
	swapBackend transaction.Backend,
	chainId int64,
	overlayEthAddress common.Address,
	erc20BindingFunc ERC20BindingFunc,
	erc20Address common.Address,
	backoffDuration time.Duration,
	maxRetries uint64,
) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, backoffDuration*time.Duration(maxRetries))
	defer cancel()

	erc20Token, err := erc20BindingFunc(erc20Address, swapBackend)
	if err != nil {
		return err
	}

	for {
		balance, err := erc20Token.BalanceOf(&bind.CallOpts{
			Context: timeoutCtx,
		}, overlayEthAddress)
		if err != nil {
			return err
		}

		if balance.Cmp(swapInitialDeposit) < 0 {
			logger.Warningf("cannot continue until there is sufficient ETH and BZZ available on %x", overlayEthAddress)
			if chainId == 5 {
				logger.Warningf("on the test network you can get both Goerli ETH and Goerli BZZ from https://faucet.ethswarm.org?address=%x", overlayEthAddress)
			}
			select {
			case <-time.After(backoffDuration):
			case <-timeoutCtx.Done():
				return fmt.Errorf("insufficient token for initial deposit")
			}
			continue
		}

		return nil
	}
}

// Init initialises the chequebook service.
func Init(
	ctx context.Context,
	chequebookFactory Factory,
	stateStore storage.StateStorer,
	logger logging.Logger,
	swapInitialDeposit *big.Int,
	transactionService transaction.Service,
	swapBackend transaction.Backend,
	chainId int64,
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
		if swapInitialDeposit.Cmp(big.NewInt(0)) != 0 {
			err = checkBalance(ctx, logger, swapInitialDeposit, swapBackend, chainId, overlayEthAddress, erc20BindingFunc, erc20Address, 20*time.Second, 10)
			if err != nil {
				return nil, err
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

		if swapInitialDeposit.Cmp(big.NewInt(0)) != 0 {
			logger.Infof("depositing %d token into new chequebook", swapInitialDeposit)
			depositHash, err := chequebookService.Deposit(ctx, swapInitialDeposit)
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
