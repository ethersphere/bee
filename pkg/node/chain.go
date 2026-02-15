// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethersphere/bee/v2/pkg/config"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/v2/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/v2/pkg/settlement"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/erc20"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/priceoracle"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/swapprotocol"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/transaction/backendnoop"
	"github.com/ethersphere/bee/v2/pkg/transaction/wrapped"
)

const (
	maxDelay                = 1 * time.Minute
	cancellationDepth       = 12
	additionalConfirmations = 2
)

// InitChain will initialize the Ethereum backend at the given endpoint and
// set up the Transaction Service to interact with it using the provided signer.
func InitChain(
	ctx context.Context,
	logger log.Logger,
	stateStore storage.StateStorer,
	endpoint string,
	chainID int64,
	signer crypto.Signer,
	pollingInterval time.Duration,
	chainEnabled bool,
	minimumGasTipCap uint64,
	fallbackGasLimit uint64,
) (transaction.Backend, common.Address, int64, transaction.Monitor, transaction.Service, error) {
	backend := backendnoop.New(chainID)

	if chainEnabled {
		rpcClient, err := rpc.DialContext(ctx, endpoint)
		if err != nil {
			return nil, common.Address{}, 0, nil, nil, fmt.Errorf("dial blockchain client: %w", err)
		}

		var versionString string
		if err = rpcClient.CallContext(ctx, &versionString, "web3_clientVersion"); err != nil {
			logger.Info("could not connect to backend; "+
				"in a swap-enabled network a working blockchain node "+
				"(for xDAI network in production, SepoliaETH in testnet) is required; "+
				"check your node or specify another node using --blockchain-rpc-endpoint.",
				"blockchain-rpc-endpoint", endpoint)
			return nil, common.Address{}, 0, nil, nil, fmt.Errorf("get client version: %w", err)
		}

		logger.Info("connected to blockchain backend", "version", versionString)

		backend = wrapped.NewBackend(ethclient.NewClient(rpcClient), minimumGasTipCap)
	}

	backendChainID, err := backend.ChainID(ctx)
	if err != nil {
		return nil, common.Address{}, 0, nil, nil, fmt.Errorf("getting chain id: %w", err)
	}

	if chainID != -1 && chainID != backendChainID.Int64() {
		return nil, common.Address{}, 0, nil, nil, fmt.Errorf("connected to wrong network: expected chain id %d, got %d", chainID, backendChainID.Int64())
	}

	overlayEthAddress, err := signer.EthereumAddress()
	if err != nil {
		return nil, common.Address{}, 0, nil, nil, fmt.Errorf("blockchain address: %w", err)
	}

	transactionMonitor := transaction.NewMonitor(logger, backend, overlayEthAddress, pollingInterval, cancellationDepth)

	transactionService, err := transaction.NewService(logger, overlayEthAddress, backend, signer, stateStore, backendChainID, transactionMonitor, fallbackGasLimit)
	if err != nil {
		return nil, common.Address{}, 0, nil, nil, fmt.Errorf("transaction service: %w", err)
	}

	return backend, overlayEthAddress, backendChainID.Int64(), transactionMonitor, transactionService, nil
}

// InitChequebookFactory will initialize the chequebook factory with the given
// chain backend.
func InitChequebookFactory(logger log.Logger, backend transaction.Backend, chainID int64, transactionService transaction.Service, factoryAddress string) (chequebook.Factory, error) {
	var currentFactory common.Address

	chainCfg, found := config.GetByChainID(chainID)

	foundFactory := chainCfg.CurrentFactoryAddress
	if factoryAddress == "" {
		if !found {
			return nil, fmt.Errorf("no known factory address for this network (chain id: %d)", chainID)
		}
		currentFactory = foundFactory
		logger.Info("using default factory address", "chain_id", chainID, "factory_address", currentFactory)
	} else if !common.IsHexAddress(factoryAddress) {
		return nil, errors.New("malformed factory address")
	} else {
		currentFactory = common.HexToAddress(factoryAddress)
		logger.Info("using custom factory address", "factory_address", currentFactory)
	}

	return chequebook.NewFactory(backend, transactionService, currentFactory), nil
}

// InitChequebookService will initialize the chequebook service with the given
// chequebook factory and chain backend.
func InitChequebookService(
	ctx context.Context,
	logger log.Logger,
	stateStore storage.StateStorer,
	signer crypto.Signer,
	chainID int64,
	backend transaction.Backend,
	overlayEthAddress common.Address,
	transactionService transaction.Service,
	chequebookFactory chequebook.Factory,
	initialDeposit string,
	erc20Service erc20.Service,
) (chequebook.Service, error) {
	chequeSigner := chequebook.NewChequeSigner(signer, chainID)

	deposit, ok := new(big.Int).SetString(initialDeposit, 10)
	if !ok {
		return nil, fmt.Errorf("initial swap deposit \"%s\" cannot be parsed", initialDeposit)
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
		erc20Service,
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
	logger log.Logger,
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
		chainCfg, found := config.GetByChainID(chainID)
		currentPriceOracleAddress = chainCfg.SwapPriceOracleAddress
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

	cashoutAddress := overlayEthAddress
	if chequebookService != nil {
		cashoutAddress = chequebookService.Address()
	}

	swapService := swap.New(
		swapProtocol,
		logger,
		stateStore,
		chequebookService,
		chequeStore,
		swapAddressBook,
		networkID,
		cashoutService,
		accounting,
		cashoutAddress,
	)

	swapProtocol.SetSwap(swapService)

	err := p2ps.AddProtocol(swapProtocol.Protocol())
	if err != nil {
		return nil, nil, err
	}

	return swapService, priceOracle, nil
}

func GetTxHash(stateStore storage.StateStorer, logger log.Logger, trxString string) ([]byte, error) {
	if trxString != "" {
		txHashTrimmed := strings.TrimPrefix(trxString, "0x")
		if len(txHashTrimmed) != 64 {
			return nil, errors.New("invalid length")
		}
		txHash, err := hex.DecodeString(txHashTrimmed)
		if err != nil {
			return nil, err
		}
		logger.Info("using the provided transaction hash", "tx_hash", txHashTrimmed)
		return txHash, nil
	}

	var txHash common.Hash
	key := chequebook.ChequebookDeploymentKey
	if err := stateStore.Get(key, &txHash); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, errors.New("chequebook deployment transaction hash not found, please specify the transaction hash manually")
		}
		return nil, err
	}

	logger.Info("using the chequebook transaction hash", "tx_hash", txHash)
	return txHash.Bytes(), nil
}

func GetTxNextBlock(ctx context.Context, logger log.Logger, backend transaction.Backend, monitor transaction.Monitor, duration time.Duration, trx []byte, blockHash string) ([]byte, error) {
	if blockHash != "" {
		blockHashTrimmed := strings.TrimPrefix(blockHash, "0x")
		if len(blockHashTrimmed) != 64 {
			return nil, errors.New("invalid length")
		}
		blockHash, err := hex.DecodeString(blockHashTrimmed)
		if err != nil {
			return nil, err
		}
		logger.Info("using the provided block hash", "block_hash", hex.EncodeToString(blockHash))
		return blockHash, nil
	}

	block, err := transaction.WaitBlockAfterTransaction(ctx, backend, duration, common.BytesToHash(trx), additionalConfirmations)
	if err != nil {
		return nil, err
	}

	hash := block.Hash()
	hashBytes := hash.Bytes()

	logger.Info("using the next block hash from the blockchain", "block_hash", hex.EncodeToString(hashBytes))

	return hashBytes, nil
}

// noOpChequebookService is a noOp implementation for chequebook.Service interface.
type noOpChequebookService struct{}

func (m *noOpChequebookService) Deposit(context.Context, *big.Int) (hash common.Hash, err error) {
	return hash, postagecontract.ErrChainDisabled
}

func (m *noOpChequebookService) Withdraw(context.Context, *big.Int) (hash common.Hash, err error) {
	return hash, postagecontract.ErrChainDisabled
}

func (m *noOpChequebookService) WaitForDeposit(context.Context, common.Hash) error {
	return postagecontract.ErrChainDisabled
}

func (m *noOpChequebookService) Balance(context.Context) (*big.Int, error) {
	return nil, postagecontract.ErrChainDisabled
}

func (m *noOpChequebookService) AvailableBalance(context.Context) (*big.Int, error) {
	return nil, postagecontract.ErrChainDisabled
}

func (m *noOpChequebookService) Address() common.Address {
	return common.Address{}
}

func (m *noOpChequebookService) Issue(context.Context, common.Address, *big.Int, chequebook.SendChequeFunc) (*big.Int, error) {
	return nil, postagecontract.ErrChainDisabled
}

func (m *noOpChequebookService) LastCheque(common.Address) (*chequebook.SignedCheque, error) {
	return nil, postagecontract.ErrChainDisabled
}

func (m *noOpChequebookService) LastCheques() (map[common.Address]*chequebook.SignedCheque, error) {
	return nil, postagecontract.ErrChainDisabled
}
