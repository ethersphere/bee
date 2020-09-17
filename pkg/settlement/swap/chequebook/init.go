package chequebook

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
)

func Init(
	p2pCtx context.Context,
	chequebookFactory Factory,
	stateStore storage.StateStorer,
	logger logging.Logger,
	swapInitialDeposit uint64,
	transactionService TransactionService,
	swapBackend Backend,
	overlayEthAddress common.Address) (chequebookService Service, err error) {
	err = chequebookFactory.VerifyBytecode(p2pCtx)
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

		chequebookAddress, err = chequebookFactory.Deploy(p2pCtx, overlayEthAddress, big.NewInt(0))
		if err != nil {
			return nil, err
		}

		logger.Infof("deployed to address %x", chequebookAddress)

		err = stateStore.Put("chequebook", chequebookAddress)
		if err != nil {
			return nil, err
		}

		chequebookService, err = New(swapBackend, transactionService, chequebookAddress)
		if err != nil {
			return nil, err
		}

		if swapInitialDeposit != 0 {
			logger.Info("depositing into new chequebook")

			depositHash, err := chequebookService.Deposit(p2pCtx, big.NewInt(int64(swapInitialDeposit)))
			if err != nil {
				return nil, err
			}

			err = chequebookService.WaitForDeposit(p2pCtx, depositHash)
			if err != nil {
				return nil, err
			}

			logger.Infof("deposited to chequebook %x in transaction %x", chequebookAddress, depositHash)
		}
	} else {
		chequebookService, err = New(swapBackend, transactionService, chequebookAddress)
		if err != nil {
			return nil, err
		}

		logger.Infof("using existing chequebook %x", chequebookAddress)
	}

	err = chequebookFactory.VerifyChequebook(p2pCtx, chequebookService.Address())
	if err != nil {
		return nil, err
	}

	return chequebookService, nil
}
