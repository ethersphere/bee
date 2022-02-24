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

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethersphere/bee/pkg/config"
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
	"github.com/ethersphere/bee/pkg/transaction/wrapped"
	"github.com/ethersphere/go-sw3-abi/sw3abi"
	"github.com/prometheus/client_golang/prometheus"
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
	logger logging.Logger,
	stateStore storage.StateStorer,
	endpoint string,
	oChainID int64,
	signer crypto.Signer,
	pollingInterval time.Duration,
	chainEnabled bool,
) (transaction.Backend, common.Address, int64, transaction.Monitor, transaction.Service, error) {
	var backend transaction.Backend = &noOpChainBackend{
		chainID: oChainID,
	}

	if chainEnabled {
		// connect to the real one
		rpcClient, err := rpc.DialContext(ctx, endpoint)
		if err != nil {
			return nil, common.Address{}, 0, nil, nil, fmt.Errorf("dial eth client: %w", err)
		}

		var versionString string
		err = rpcClient.CallContext(ctx, &versionString, "web3_clientVersion")
		if err != nil {
			logger.Infof("could not connect to backend at %v. In a swap-enabled network a working blockchain node (for xdai network in production, goerli in testnet) is required. Check your node or specify another node using --swap-endpoint.", endpoint)
			return nil, common.Address{}, 0, nil, nil, fmt.Errorf("eth client get version: %w", err)
		}

		logger.Infof("connected to ethereum backend: %s", versionString)

		backend = wrapped.NewBackend(ethclient.NewClient(rpcClient))
	}

	chainID, err := backend.ChainID(ctx)
	if err != nil {
		return nil, common.Address{}, 0, nil, nil, fmt.Errorf("get chain id: %w", err)
	}

	overlayEthAddress, err := signer.EthereumAddress()
	if err != nil {
		return nil, common.Address{}, 0, nil, nil, fmt.Errorf("eth address: %w", err)
	}

	transactionMonitor := transaction.NewMonitor(logger, backend, overlayEthAddress, pollingInterval, cancellationDepth)

	transactionService, err := transaction.NewService(logger, backend, signer, stateStore, chainID, transactionMonitor)
	if err != nil {
		return nil, common.Address{}, 0, nil, nil, fmt.Errorf("new transaction service: %w", err)
	}

	return backend, overlayEthAddress, chainID.Int64(), transactionMonitor, transactionService, nil
}

// InitChequebookFactory will initialize the chequebook factory with the given
// chain backend.
func InitChequebookFactory(
	logger logging.Logger,
	backend transaction.Backend,
	chainID int64,
	transactionService transaction.Service,
	factoryAddress string,
	legacyFactoryAddresses []string,
) (chequebook.Factory, error) {
	var currentFactory common.Address
	var legacyFactories []common.Address

	chainCfg, found := config.GetChainConfig(chainID)

	foundFactory, foundLegacyFactories := chainCfg.CurrentFactory, chainCfg.LegacyFactories
	if factoryAddress == "" {
		if !found {
			return nil, fmt.Errorf("no known factory address for this network (chain id: %d)", chainID)
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
	backend transaction.Backend,
	overlayEthAddress common.Address,
	transactionService transaction.Service,
	chequebookFactory chequebook.Factory,
	initialDeposit string,
	deployGasPrice string,
	chainEnabled bool,
) (chequebook.Service, error) {
	if !chainEnabled {
		return new(noOpChequebookService), nil
	}

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
		chainCfg, found := config.GetChainConfig(chainID)
		currentPriceOracleAddress = chainCfg.PriceOracleAddress
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

func GetTxHash(stateStore storage.StateStorer, logger logging.Logger, trxString string) ([]byte, error) {

	if trxString != "" {
		txHashTrimmed := strings.TrimPrefix(trxString, "0x")
		if len(txHashTrimmed) != 64 {
			return nil, errors.New("invalid length")
		}
		txHash, err := hex.DecodeString(txHashTrimmed)
		if err != nil {
			return nil, err
		}
		logger.Infof("using the provided transaction hash %x", txHash)
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

	logger.Infof("using the chequebook transaction hash %x", txHash)
	return txHash.Bytes(), nil
}

func GetTxNextBlock(ctx context.Context, logger logging.Logger, backend transaction.Backend, monitor transaction.Monitor, duration time.Duration, trx []byte, blockHash string) ([]byte, error) {

	if blockHash != "" {
		blockHashTrimmed := strings.TrimPrefix(blockHash, "0x")
		if len(blockHashTrimmed) != 64 {
			return nil, errors.New("invalid length")
		}
		blockHash, err := hex.DecodeString(blockHashTrimmed)
		if err != nil {
			return nil, err
		}
		logger.Infof("using the provided block hash %x", blockHash)
		return blockHash, nil
	}

	block, err := transaction.WaitBlockAfterTransaction(ctx, backend, duration, common.BytesToHash(trx), additionalConfirmations)
	if err != nil {
		return nil, err
	}

	hash := block.Hash()
	hashBytes := hash.Bytes()

	logger.Infof("using the next block hash from the blockchain %x", hashBytes)

	return hashBytes, nil
}

// noOpChequebookService is a noOp implementation for chequebook.Service interface.
type noOpChequebookService struct{}

func (m *noOpChequebookService) Deposit(context.Context, *big.Int) (hash common.Hash, err error) {
	return hash, errors.New("chain disabled")
}
func (m *noOpChequebookService) Withdraw(context.Context, *big.Int) (hash common.Hash, err error) {
	return hash, errors.New("chain disabled")
}
func (m *noOpChequebookService) WaitForDeposit(context.Context, common.Hash) error {
	return errors.New("chain disabled")
}
func (m *noOpChequebookService) Balance(context.Context) (*big.Int, error) {
	return nil, errors.New("chain disabled")
}
func (m *noOpChequebookService) AvailableBalance(context.Context) (*big.Int, error) {
	return nil, errors.New("chain disabled")
}
func (m *noOpChequebookService) Address() common.Address {
	return common.Address{}
}
func (m *noOpChequebookService) Issue(context.Context, common.Address, *big.Int, chequebook.SendChequeFunc) (*big.Int, error) {
	return nil, errors.New("chain disabled")
}
func (m *noOpChequebookService) LastCheque(common.Address) (*chequebook.SignedCheque, error) {
	return nil, errors.New("chain disabled")
}
func (m *noOpChequebookService) LastCheques() (map[common.Address]*chequebook.SignedCheque, error) {
	return nil, errors.New("chain disabled")
}

// noOpChainBackend is a noOp implementation for transaction.Backend interface.
type noOpChainBackend struct {
	chainID int64
}

func (m noOpChainBackend) Metrics() []prometheus.Collector {
	return nil
}

func (m noOpChainBackend) CodeAt(context.Context, common.Address, *big.Int) ([]byte, error) {
	return common.FromHex(sw3abi.SimpleSwapFactoryDeployedBinv0_4_0), nil
}
func (m noOpChainBackend) CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) {
	panic("chain no op: CallContract")
}
func (m noOpChainBackend) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	h := new(types.Header)
	h.Time = uint64(time.Now().Unix())
	return h, nil
}
func (m noOpChainBackend) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	panic("chain no op: PendingNonceAt")
}
func (m noOpChainBackend) SuggestGasPrice(context.Context) (*big.Int, error) {
	panic("chain no op: SuggestGasPrice")
}
func (m noOpChainBackend) EstimateGas(context.Context, ethereum.CallMsg) (uint64, error) {
	panic("chain no op: EstimateGas")
}
func (m noOpChainBackend) SendTransaction(context.Context, *types.Transaction) error {
	panic("chain no op: SendTransaction")
}
func (m noOpChainBackend) TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error) {
	r := new(types.Receipt)
	r.BlockNumber = big.NewInt(1)
	return r, nil
}
func (m noOpChainBackend) TransactionByHash(context.Context, common.Hash) (tx *types.Transaction, isPending bool, err error) {
	panic("chain no op: TransactionByHash")
}
func (m noOpChainBackend) BlockNumber(context.Context) (uint64, error) {
	return 4, nil
}
func (m noOpChainBackend) BalanceAt(context.Context, common.Address, *big.Int) (*big.Int, error) {
	panic("chain no op: BalanceAt")
}
func (m noOpChainBackend) NonceAt(context.Context, common.Address, *big.Int) (uint64, error) {
	panic("chain no op: NonceAt")
}
func (m noOpChainBackend) FilterLogs(context.Context, ethereum.FilterQuery) ([]types.Log, error) {
	panic("chain no op: FilterLogs")
}
func (m noOpChainBackend) ChainID(context.Context) (*big.Int, error) {
	return big.NewInt(m.chainID), nil
}
func (m noOpChainBackend) Close() {}
