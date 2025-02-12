// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	chaincfg "github.com/ethersphere/bee/v2/pkg/config"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/sctx"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/erc20"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/transaction"
)

const (
	chequebookKey           = "swap_chequebook"
	ChequebookDeploymentKey = "swap_chequebook_transaction_deployment"

	balanceCheckBackoffDuration = 20 * time.Second
	balanceCheckMaxRetries      = 10
)

const (
	erc20SmallUnitStr = "10000000000000000"
	ethSmallUnitStr   = "1000000000000000000"
)

func checkBalance(
	ctx context.Context,
	logger log.Logger,
	swapInitialDeposit *big.Int,
	swapBackend transaction.Backend,
	chainId int64,
	overlayEthAddress common.Address,
	erc20Token erc20.Service,
) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, balanceCheckBackoffDuration*time.Duration(balanceCheckMaxRetries))
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

		gasPrice := sctx.GetGasPrice(ctx)
		minimumEth := big.NewInt(0)

		if gasPrice == nil {
			gasPrice, err = swapBackend.SuggestGasPrice(timeoutCtx)
			if err != nil {
				return err
			}

			minimumEth = gasPrice.Mul(gasPrice, big.NewInt(250000))
		}

		insufficientERC20 := erc20Balance.Cmp(swapInitialDeposit) < 0
		insufficientETH := ethBalance.Cmp(minimumEth) < 0

		erc20SmallUnit, ethSmallUnit := new(big.Int), new(big.Float)
		erc20SmallUnit.SetString(erc20SmallUnitStr, 10)
		ethSmallUnit.SetString(ethSmallUnitStr)

		if insufficientERC20 || insufficientETH {
			neededERC20, mod := new(big.Int).DivMod(swapInitialDeposit, erc20SmallUnit, new(big.Int))
			if mod.Cmp(big.NewInt(0)) > 0 {
				// always round up the division as the bzzaar cannot handle decimals
				neededERC20.Add(neededERC20, big.NewInt(1))
			}

			neededETH := new(big.Float).Quo(new(big.Float).SetInt(minimumEth), ethSmallUnit)

			ccfg, _ := chaincfg.GetByChainID(chainId)
			swarmTokenName := ccfg.SwarmTokenSymbol
			nativeTokenName := ccfg.NativeTokenSymbol

			if insufficientETH && insufficientERC20 {
				msg := fmt.Sprintf("cannot continue until there is at least min %s (for Gas) and at least min %s available on address", nativeTokenName, swarmTokenName)
				logger.Warning(msg, "min_amount", neededETH, "min_bzz_amount", neededERC20, "address", overlayEthAddress)
			} else if insufficientETH {
				msg := fmt.Sprintf("cannot continue until there is at least min %s (for Gas) available on address", nativeTokenName)
				logger.Warning(msg, "min_amount", neededETH, "address", overlayEthAddress)
			} else {
				msg := fmt.Sprintf("cannot continue until there is at least min %s available on address", swarmTokenName)
				logger.Warning(msg, "min_amount", neededERC20, "address", overlayEthAddress)
			}
			if chainId == chaincfg.Testnet.ChainID {
				logger.Warning("learn how to fund your node by visiting our docs at https://docs.ethswarm.org/docs/installation/fund-your-node")
			}
			select {
			case <-time.After(balanceCheckBackoffDuration):
			case <-timeoutCtx.Done():
				if insufficientERC20 {
					return fmt.Errorf("insufficient %s for initial deposit", swarmTokenName)
				} else {
					return fmt.Errorf("insufficient %s for initial deposit", nativeTokenName)
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
	logger log.Logger,
	swapInitialDeposit *big.Int,
	transactionService transaction.Service,
	swapBackend transaction.Backend,
	chainId int64,
	overlayEthAddress common.Address,
	chequeSigner ChequeSigner,
	erc20Service erc20.Service,
) (chequebookService Service, err error) {
	logger = logger.WithName(loggerName).Register()

	var chequebookAddress common.Address
	err = stateStore.Get(chequebookKey, &chequebookAddress)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}

		var txHash common.Hash
		err = stateStore.Get(ChequebookDeploymentKey, &txHash)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
		if errors.Is(err, storage.ErrNotFound) {
			logger.Info("no chequebook found, deploying new one.")
			err = checkBalance(ctx, logger, swapInitialDeposit, swapBackend, chainId, overlayEthAddress, erc20Service)
			if err != nil {
				return nil, err
			}

			nonce := make([]byte, 32)
			_, err = rand.Read(nonce)
			if err != nil {
				return nil, err
			}

			// if we don't yet have a chequebook, deploy a new one
			txHash, err = chequebookFactory.Deploy(ctx, overlayEthAddress, big.NewInt(0), common.BytesToHash(nonce))
			if err != nil {
				return nil, err
			}

			logger.Info("deploying new chequebook", "tx", txHash)

			err = stateStore.Put(ChequebookDeploymentKey, txHash)
			if err != nil {
				return nil, err
			}
		} else {
			logger.Info("waiting for chequebook deployment", "tx", txHash)
		}

		chequebookAddress, err = chequebookFactory.WaitDeployed(ctx, txHash)
		if err != nil {
			return nil, err
		}

		logger.Info("chequebook deployed", "chequebook_address", chequebookAddress)

		// save the address for later use
		err = stateStore.Put(chequebookKey, chequebookAddress)
		if err != nil {
			return nil, err
		}

		chequebookService, err = New(transactionService, chequebookAddress, overlayEthAddress, stateStore, chequeSigner, erc20Service)
		if err != nil {
			return nil, err
		}

		if swapInitialDeposit.Cmp(big.NewInt(0)) != 0 {
			logger.Info("depositing token into new chequebook", "amount", swapInitialDeposit)
			depositHash, err := chequebookService.Deposit(ctx, swapInitialDeposit)
			if err != nil {
				return nil, err
			}

			logger.Info("sent deposit transaction", "tx", depositHash)
			err = chequebookService.WaitForDeposit(ctx, depositHash)
			if err != nil {
				return nil, err
			}

			logger.Info("successfully deposited to chequebook")
		}
	} else {
		chequebookService, err = New(transactionService, chequebookAddress, overlayEthAddress, stateStore, chequeSigner, erc20Service)
		if err != nil {
			return nil, err
		}

		logger.Info("using existing chequebook", "chequebook_address", chequebookAddress)
	}

	// regardless of how the chequebook service was initialised make sure that the chequebook is valid
	err = chequebookFactory.VerifyChequebook(ctx, chequebookService.Address())
	if err != nil {
		return nil, err
	}

	return chequebookService, nil
}
