// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/settlement/swap/erc20"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction"
	"github.com/ethersphere/bee/pkg/storage"
)

const (
	chequebookKey           = "swap_chequebook"
	chequebookDeploymentKey = "swap_chequebook_transaction_deployment"
)

func checkBalance(
	ctx context.Context,
	logger logging.Logger,
	swapInitialDeposit *big.Int,
	swapBackend transaction.Backend,
	chainId int64,
	overlayEthAddress common.Address,
	erc20Token erc20.Service,
	backoffDuration time.Duration,
	maxRetries uint64,
) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, backoffDuration*time.Duration(maxRetries))
	defer cancel()
	for {
		erc20Balance, err := erc20Token.BalanceOf(timeoutCtx, overlayEthAddress)
		if err != nil {
			return err
		}

		ethBalance, err := swapBackend.BalanceAt(timeoutCtx, overlayEthAddress, nil)
		if err != nil {
			return err
		}

		gasPrice, err := swapBackend.SuggestGasPrice(timeoutCtx)
		if err != nil {
			return err
		}

		minimumEth := gasPrice.Mul(gasPrice, big.NewInt(2000000))

		insufficientERC20 := erc20Balance.Cmp(swapInitialDeposit) < 0
		insufficientETH := ethBalance.Cmp(minimumEth) < 0

		if insufficientERC20 || insufficientETH {
			neededERC20, mod := new(big.Int).DivMod(swapInitialDeposit, big.NewInt(10000000000000000), new(big.Int))
			if mod.Cmp(big.NewInt(0)) > 0 {
				// always round up the division as the bzzaar cannot handle decimals
				neededERC20.Add(neededERC20, big.NewInt(1))
			}

			if insufficientETH && insufficientERC20 {
				logger.Warningf("cannot continue until there is sufficient ETH (for Gas) and at least %d BZZ available on %x", neededERC20, overlayEthAddress)
			} else if insufficientETH {
				logger.Warningf("cannot continue until there is sufficient ETH (for Gas) available on %x", overlayEthAddress)
			} else {
				logger.Warningf("cannot continue until there is at least %d BZZ available on %x", neededERC20, overlayEthAddress)
			}
			if chainId == 5 {
				logger.Warningf("get your Goerli ETH and Goerli BZZ now via the bzzaar at https://bzz.ethswarm.org/?transaction=buy&amount=%d&slippage=30&receiver=0x%x", neededERC20, overlayEthAddress)
			}
			select {
			case <-time.After(backoffDuration):
			case <-timeoutCtx.Done():
				if insufficientERC20 {
					return fmt.Errorf("insufficient BZZ for initial deposit")
				} else {
					return fmt.Errorf("insufficient ETH for initial deposit")
				}
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
) (chequebookService Service, err error) {
	// verify that the supplied factory is valid
	err = chequebookFactory.VerifyBytecode(ctx)
	if err != nil {
		return nil, err
	}

	erc20Address, err := chequebookFactory.ERC20Address(ctx)
	if err != nil {
		return nil, err
	}

	erc20Service := erc20.New(swapBackend, transactionService, erc20Address)

	var chequebookAddress common.Address
	err = stateStore.Get(chequebookKey, &chequebookAddress)
	if err != nil {
		if err != storage.ErrNotFound {
			return nil, err
		}

		var txHash common.Hash
		err = stateStore.Get(chequebookDeploymentKey, &txHash)
		if err != nil && err != storage.ErrNotFound {
			return nil, err
		}
		if err == storage.ErrNotFound {
			logger.Info("no chequebook found, deploying new one.")
			if swapInitialDeposit.Cmp(big.NewInt(0)) != 0 {
				err = checkBalance(ctx, logger, swapInitialDeposit, swapBackend, chainId, overlayEthAddress, erc20Service, 20*time.Second, 10)
				if err != nil {
					return nil, err
				}
			}

			// if we don't yet have a chequebook, deploy a new one
			txHash, err = chequebookFactory.Deploy(ctx, overlayEthAddress, big.NewInt(0))
			if err != nil {
				return nil, err
			}

			logger.Infof("deploying new chequebook in transaction %x", txHash)

			err = stateStore.Put(chequebookDeploymentKey, txHash)
			if err != nil {
				return nil, err
			}
		} else {
			logger.Infof("waiting for chequebook deployment in transaction %x", txHash)
		}

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

		chequebookService, err = New(swapBackend, transactionService, chequebookAddress, overlayEthAddress, stateStore, chequeSigner, erc20Service, simpleSwapBindingFunc)
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
		chequebookService, err = New(swapBackend, transactionService, chequebookAddress, overlayEthAddress, stateStore, chequeSigner, erc20Service, simpleSwapBindingFunc)
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
