// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/settlement"
	"github.com/ethersphere/bee/pkg/settlement/swap"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/settlement/swap/priceoracle"
	"github.com/ethersphere/bee/pkg/settlement/swap/swapprotocol"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/transaction"
)

const (
	maxDelay          = 1 * time.Minute
	cancellationDepth = 6
)

// InitChain will initialize the Ethereum backend at the given endpoint and
// set up the Transaction Service to interact with it using the provided signer.
func InitChain(
	ctx context.Context,
	logger logging.Logger,
	stateStore storage.StateStorer,
	endpoint string,
	signer crypto.Signer,
	blocktime uint64,
) (*ethclient.Client, common.Address, int64, transaction.Monitor, transaction.Service, error) {
	backend, err := ethclient.Dial(endpoint)
	if err != nil {
		return nil, common.Address{}, 0, nil, nil, fmt.Errorf("dial eth client: %w", err)
	}

	chainID, err := backend.ChainID(ctx)
	if err != nil {
		logger.Infof("could not connect to backend at %v. In a swap-enabled network a working blockchain node (for goerli network in production) is required. Check your node or specify another node using --swap-endpoint.", endpoint)
		return nil, common.Address{}, 0, nil, nil, fmt.Errorf("get chain id: %w", err)
	}

	pollingInterval := time.Duration(blocktime) * time.Second
	overlayEthAddress, err := signer.EthereumAddress()
	if err != nil {
		return nil, common.Address{}, 0, nil, nil, fmt.Errorf("eth address: %w", err)
	}

	transactionMonitor := transaction.NewMonitor(logger, backend, overlayEthAddress, pollingInterval, cancellationDepth)

	transactionService, err := transaction.NewService(logger, backend, signer, stateStore, chainID, transactionMonitor)
	if err != nil {
		return nil, common.Address{}, 0, nil, nil, fmt.Errorf("new transaction service: %w", err)
	}

	// Sync the with the given Ethereum backend:
	isSynced, err := transaction.IsSynced(ctx, backend, maxDelay)
	if err != nil {
		return nil, common.Address{}, 0, nil, nil, fmt.Errorf("is synced: %w", err)
	}
	if !isSynced {
		logger.Infof("waiting to sync with the Ethereum backend")
		err := transaction.WaitSynced(ctx, backend, maxDelay)
		if err != nil {
			return nil, common.Address{}, 0, nil, nil, fmt.Errorf("waiting backend sync: %w", err)
		}
	}
	return backend, overlayEthAddress, chainID.Int64(), transactionMonitor, transactionService, nil
}

// InitChequebookFactory will initialize the chequebook factory with the given
// chain backend.
func InitChequebookFactory(
	logger logging.Logger,
	backend *ethclient.Client,
	chainID int64,
	transactionService transaction.Service,
	factoryAddress string,
	legacyFactoryAddresses []string,
) (chequebook.Factory, error) {
	var currentFactory common.Address
	var legacyFactories []common.Address

	foundFactory, foundLegacyFactories, found := chequebook.DiscoverFactoryAddress(chainID)
	if factoryAddress == "" {
		if !found {
			return nil, errors.New("no known factory address for this network")
		}
		currentFactory = foundFactory
		logger.Infof("using default factory address for chain id %d: %x", chainID, currentFactory)
	} else if !common.IsHexAddress(factoryAddress) {
		return nil, errors.New("malformed factory address")
	} else {
		currentFactory = common.HexToAddress(factoryAddress)
		logger.Infof("using custom factory address: %x", currentFactory)
	}

	if len(legacyFactoryAddresses) == 0 {
		if found {
			legacyFactories = foundLegacyFactories
		}
	} else {
		for _, legacyAddress := range legacyFactoryAddresses {
			if !common.IsHexAddress(legacyAddress) {
				return nil, errors.New("malformed factory address")
			}
			legacyFactories = append(legacyFactories, common.HexToAddress(legacyAddress))
		}
	}

	return chequebook.NewFactory(
		backend,
		transactionService,
		currentFactory,
		legacyFactories,
	), nil
}

// InitChequebookService will initialize the chequebook service with the given
// chequebook factory and chain backend.
func InitChequebookService(
	ctx context.Context,
	logger logging.Logger,
	stateStore storage.StateStorer,
	signer crypto.Signer,
	chainID int64,
	backend *ethclient.Client,
	overlayEthAddress common.Address,
	transactionService transaction.Service,
	chequebookFactory chequebook.Factory,
	initialDeposit string,
	deployGasPrice string,
) (chequebook.Service, error) {
	chequeSigner := chequebook.NewChequeSigner(signer, chainID)

	deposit, ok := new(big.Int).SetString(initialDeposit, 10)
	if !ok {
		return nil, fmt.Errorf("initial swap deposit \"%s\" cannot be parsed", initialDeposit)
	}

	if deployGasPrice != "" {
		gasPrice, ok := new(big.Int).SetString(deployGasPrice, 10)
		if !ok {
			return nil, fmt.Errorf("deploy gas price \"%s\" cannot be parsed", deployGasPrice)
		}
		ctx = sctx.SetGasPrice(ctx, gasPrice)
	}

	chequebookService, err := chequebook.Init(
		ctx,
		chequebookFactory,
		stateStore,
		logger,
		deposit,
		transactionService,
		backend,
		chainID,
		overlayEthAddress,
		chequeSigner,
	)
	if err != nil {
		return nil, fmt.Errorf("chequebook init: %w", err)
	}

	return chequebookService, nil
}

func initChequeStoreCashout(
	stateStore storage.StateStorer,
	swapBackend transaction.Backend,
	chequebookFactory chequebook.Factory,
	chainID int64,
	overlayEthAddress common.Address,
	transactionService transaction.Service,
) (chequebook.ChequeStore, chequebook.CashoutService) {
	chequeStore := chequebook.NewChequeStore(
		stateStore,
		chequebookFactory,
		chainID,
		overlayEthAddress,
		transactionService,
		chequebook.RecoverCheque,
	)

	cashout := chequebook.NewCashoutService(
		stateStore,
		swapBackend,
		transactionService,
		chequeStore,
	)

	return chequeStore, cashout
}

// InitSwap will initialize and register the swap service.
func InitSwap(
	p2ps *libp2p.Service,
	logger logging.Logger,
	stateStore storage.StateStorer,
	networkID uint64,
	overlayEthAddress common.Address,
	chequebookService chequebook.Service,
	chequeStore chequebook.ChequeStore,
	cashoutService chequebook.CashoutService,
	accounting settlement.Accounting,
	priceOracleAddress string,
	chainID int64,
	transactionService transaction.Service,
) (*swap.Service, priceoracle.Service, error) {

	var currentPriceOracleAddress common.Address
	if priceOracleAddress == "" {
		var found bool
		currentPriceOracleAddress, found = priceoracle.DiscoverPriceOracleAddress(chainID)
		if !found {
			return nil, nil, errors.New("no known price oracle address for this network")
		}
	} else {
		currentPriceOracleAddress = common.HexToAddress(priceOracleAddress)
	}

	priceOracle := priceoracle.New(logger, currentPriceOracleAddress, transactionService, 300)
	priceOracle.Start()
	swapProtocol := swapprotocol.New(p2ps, logger, overlayEthAddress, priceOracle)
	swapAddressBook := swap.NewAddressbook(stateStore)

	swapService := swap.New(
		swapProtocol,
		logger,
		stateStore,
		chequebookService,
		chequeStore,
		swapAddressBook,
		networkID,
		cashoutService,
		p2ps,
		accounting,
	)

	swapProtocol.SetSwap(swapService)

	err := p2ps.AddProtocol(swapProtocol.Protocol())
	if err != nil {
		return nil, nil, err
	}

	return swapService, priceOracle, nil
}
