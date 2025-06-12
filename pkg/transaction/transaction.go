// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/sctx"
	"github.com/ethersphere/bee/v2/pkg/storage"
)

// NetworkMetrics holds network congestion and fee metrics
type NetworkMetrics struct {
	lastUpdate        time.Time
	avgBaseFee        *big.Int
	avgPriorityFee    *big.Int
	congestionRatio   float64
	blobBaseFee       *big.Int // For Pectra blob fee support
	recentGasUsage    []float64
	mutex             sync.RWMutex
}

// GasFeeStrategy defines different strategies for gas fee calculation
type GasFeeStrategy int

const (
	StrategyConservative GasFeeStrategy = iota
	StrategyStandard
	StrategyAggressive
	StrategyDynamic // Adapts based on network conditions
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "transaction"

const (
	noncePrefix              = "transaction_nonce_"
	storedTransactionPrefix  = "transaction_stored_"
	pendingTransactionPrefix = "transaction_pending_"
)

var (
	// ErrTransactionReverted denotes that the sent transaction has been
	// reverted.
	ErrTransactionReverted = errors.New("transaction reverted")
	ErrUnknownTransaction  = errors.New("unknown transaction")
	ErrAlreadyImported     = errors.New("already imported")
)

const (
	DefaultTipBoostPercent = 20
	DefaultGasLimit        = 1_000_000

	// Enhanced gas fee prediction constants
	MaxGasFeeHistoryBlocks   = 20    // Number of blocks to analyze for fee history
	BaseFeePercentileTarget  = 50    // Target percentile for base fee estimation
	PriorityFeePercentileTarget = 75 // Target percentile for priority fee estimation
	NetworkCongestionThreshold = 0.8 // Gas usage ratio threshold for congestion detection
	CongestionMultiplier     = 1.5   // Multiplier applied during high congestion
	BlobBaseFeeTarget        = 1     // Target blob base fee in wei (for future Pectra support)
	MinGasTipCap            = 1000000000 // 1 Gwei minimum tip
	MaxGasTipCap            = 50000000000 // 50 Gwei maximum tip
)

// TxRequest describes a request for a transaction that can be executed.
type TxRequest struct {
	To                   *common.Address // recipient of the transaction
	Data                 []byte          // transaction data
	GasPrice             *big.Int        // gas price or nil if suggested gas price should be used
	GasLimit             uint64          // gas limit or 0 if it should be estimated
	MinEstimatedGasLimit uint64          // minimum gas limit to use if the gas limit was estimated; it will not apply when this value is 0 or when GasLimit is not 0
	GasFeeCap            *big.Int        // adds a cap to maximum fee user is willing to pay
	Value                *big.Int        // amount of wei to send
	Description          string          // optional description
}

type StoredTransaction struct {
	To          *common.Address // recipient of the transaction
	Data        []byte          // transaction data
	GasPrice    *big.Int        // used gas price
	GasLimit    uint64          // used gas limit
	GasTipBoost int             // adds a tip for the miner for prioritizing transaction
	GasTipCap   *big.Int        // adds a cap to the tip
	GasFeeCap   *big.Int        // adds a cap to maximum fee user is willing to pay
	Value       *big.Int        // amount of wei to send
	Nonce       uint64          // used nonce
	Created     int64           // creation timestamp
	Description string          // description
}

// Service is the service to send transactions. It takes care of gas price, gas
// limit and nonce management.
type Service interface {
	io.Closer
	// Send creates a transaction based on the request (with gasprice increased by provided percentage) and sends it.
	Send(ctx context.Context, request *TxRequest, tipCapBoostPercent int) (txHash common.Hash, err error)
	// Call simulate a transaction based on the request.
	Call(ctx context.Context, request *TxRequest) (result []byte, err error)
	// WaitForReceipt waits until either the transaction with the given hash has been mined or the context is cancelled.
	// This is only valid for transaction sent by this service.
	WaitForReceipt(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error)
	// WatchSentTransaction start watching the given transaction.
	// This wraps the monitors watch function by loading the correct nonce from the store.
	// This is only valid for transaction sent by this service.
	WatchSentTransaction(txHash common.Hash) (<-chan types.Receipt, <-chan error, error)
	// StoredTransaction retrieves the stored information for the transaction
	StoredTransaction(txHash common.Hash) (*StoredTransaction, error)
	// PendingTransactions retrieves the list of all pending transaction hashes
	PendingTransactions() ([]common.Hash, error)
	// ResendTransaction resends a previously sent transaction
	// This operation can be useful if for some reason the transaction vanished from the eth networks pending pool
	ResendTransaction(ctx context.Context, txHash common.Hash) error
	// CancelTransaction cancels a previously sent transaction by double-spending its nonce with zero-transfer one
	CancelTransaction(ctx context.Context, originalTxHash common.Hash) (common.Hash, error)
	// TransactionFee retrieves the transaction fee
	TransactionFee(ctx context.Context, txHash common.Hash) (*big.Int, error)
	// UnwrapABIError tries to unwrap the ABI error if the given error is not nil.
	// The original error is wrapped together with the ABI error if it exists.
	UnwrapABIError(ctx context.Context, req *TxRequest, err error, abiErrors map[string]abi.Error) error
	// SetFeeStrategy allows changing the gas fee calculation strategy
	SetFeeStrategy(strategy GasFeeStrategy)
	// GetNetworkMetrics returns current network congestion metrics
	GetNetworkMetrics() (congestion float64, avgBaseFee *big.Int, avgTipFee *big.Int)
	// PredictOptimalGasTime predicts the optimal time to send a transaction based on network conditions
	PredictOptimalGasTime(ctx context.Context) (time.Duration, error)
	// CalculateBlobFee calculates blob fee for Pectra upgrade support
	CalculateBlobFee(ctx context.Context, blobCount int) (*big.Int, error)
	// SetLegacyMode enables/disables legacy gas calculation for backward compatibility
	SetLegacyMode(enabled bool)
}

type transactionService struct {
	wg     sync.WaitGroup
	lock   sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc

	logger         log.Logger
	backend        Backend
	signer         crypto.Signer
	sender         common.Address
	store          storage.StateStorer
	chainID        *big.Int
	monitor        Monitor
	networkMetrics *NetworkMetrics
	feeStrategy    GasFeeStrategy
	legacyMode     bool // For backward compatibility with tests
}

// NewService creates a new transaction service.
func NewService(logger log.Logger, overlayEthAddress common.Address, backend Backend, signer crypto.Signer, store storage.StateStorer, chainID *big.Int, monitor Monitor) (Service, error) {
	senderAddress, err := signer.EthereumAddress()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	t := &transactionService{
		ctx:     ctx,
		cancel:  cancel,
		logger:  logger.WithName(loggerName).WithValues("sender_address", overlayEthAddress).Register(),
		backend: backend,
		signer:  signer,
		sender:  senderAddress,
		store:   store,
		chainID: chainID,
		monitor: monitor,
		networkMetrics: &NetworkMetrics{
			avgBaseFee:     big.NewInt(0),
			avgPriorityFee: big.NewInt(0),
			blobBaseFee:    big.NewInt(BlobBaseFeeTarget),
			recentGasUsage: make([]float64, 0, MaxGasFeeHistoryBlocks),
		},
		feeStrategy: StrategyDynamic,
		legacyMode:  false,
	}

	err = t.waitForAllPendingTx()
	if err != nil {
		return nil, err
	}

	return t, nil
}

func (t *transactionService) waitForAllPendingTx() error {
	pendingTxs, err := t.PendingTransactions()
	if err != nil {
		return err
	}

	pendingTxs = t.filterPendingTransactions(t.ctx, pendingTxs)

	for _, txHash := range pendingTxs {
		t.waitForPendingTx(txHash)
	}

	return nil
}

// Send creates and signs a transaction based on the request and sends it.
func (t *transactionService) Send(ctx context.Context, request *TxRequest, boostPercent int) (txHash common.Hash, err error) {
	loggerV1 := t.logger.V(1).Register()

	t.lock.Lock()
	defer t.lock.Unlock()

	nonce, err := t.nextNonce(ctx)
	if err != nil {
		return common.Hash{}, err
	}

	tx, err := t.prepareTransaction(ctx, request, nonce, boostPercent)
	if err != nil {
		return common.Hash{}, err
	}

	signedTx, err := t.signer.SignTx(tx, t.chainID)
	if err != nil {
		return common.Hash{}, err
	}

	loggerV1.Debug("sending transaction", "tx", signedTx.Hash(), "nonce", nonce)

	err = t.backend.SendTransaction(ctx, signedTx)
	if err != nil {
		return common.Hash{}, err
	}

	txHash = signedTx.Hash()

	err = t.store.Put(storedTransactionKey(txHash), StoredTransaction{
		To:          signedTx.To(),
		Data:        signedTx.Data(),
		GasPrice:    signedTx.GasPrice(),
		GasLimit:    signedTx.Gas(),
		GasTipBoost: boostPercent,
		GasTipCap:   signedTx.GasTipCap(),
		GasFeeCap:   signedTx.GasFeeCap(),
		Value:       signedTx.Value(),
		Nonce:       signedTx.Nonce(),
		Created:     time.Now().Unix(),
		Description: request.Description,
	})
	if err != nil {
		return common.Hash{}, err
	}

	err = t.store.Put(pendingTransactionKey(txHash), struct{}{})
	if err != nil {
		return common.Hash{}, err
	}

	t.waitForPendingTx(txHash)

	return signedTx.Hash(), nil
}

func (t *transactionService) waitForPendingTx(txHash common.Hash) {
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		switch _, err := t.WaitForReceipt(t.ctx, txHash); {
		case err == nil:
			t.logger.Info("pending transaction confirmed", "tx", txHash)
			err = t.store.Delete(pendingTransactionKey(txHash))
			if err != nil {
				t.logger.Error(err, "unregistering finished pending transaction failed", "tx", txHash)
			}
		default:
			if errors.Is(err, ErrTransactionCancelled) {
				t.logger.Warning("pending transaction cancelled", "tx", txHash)
			} else {
				t.logger.Error(err, "waiting for pending transaction failed", "tx", txHash)
			}
		}
	}()
}

func (t *transactionService) Call(ctx context.Context, request *TxRequest) ([]byte, error) {
	msg := ethereum.CallMsg{
		From:     t.sender,
		To:       request.To,
		Data:     request.Data,
		GasPrice: request.GasPrice,
		Gas:      request.GasLimit,
		Value:    request.Value,
	}
	data, err := t.backend.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (t *transactionService) StoredTransaction(txHash common.Hash) (*StoredTransaction, error) {
	var tx StoredTransaction
	err := t.store.Get(storedTransactionKey(txHash), &tx)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrUnknownTransaction
		}
		return nil, err
	}
	return &tx, nil
}

// prepareTransaction creates a signable transaction based on a request.
func (t *transactionService) prepareTransaction(ctx context.Context, request *TxRequest, nonce uint64, boostPercent int) (tx *types.Transaction, err error) {
	var gasLimit uint64
	if request.GasLimit == 0 {
		gasLimit, err = t.backend.EstimateGas(ctx, ethereum.CallMsg{
			From: t.sender,
			To:   request.To,
			Data: request.Data,
		})
		if err != nil {
			t.logger.Debug("estimate gas failed", "error", err)
			gasLimit = request.MinEstimatedGasLimit
		}

		gasLimit += gasLimit / 4 // add 25% on top
		if gasLimit < request.MinEstimatedGasLimit {
			gasLimit = request.MinEstimatedGasLimit
		}
	} else {
		gasLimit = request.GasLimit
	}

	/*
		Transactions are EIP 1559 dynamic transactions where there are three fee related fields:
			1. base fee is the price that will be burned as part of the transaction.
			2. max fee is the max price we are willing to spend as gas price.
			3. max priority fee is max price want to give to the miner to prioritize the transaction.
		as an example:
		if base fee is 15, max fee is 20, and max priority is 3, gas price will be 15 + 3 = 18
		if base is 15, max fee is 20, and max priority fee is 10,
		gas price will be 15 + 10 = 25, but since 25 > 20, gas price is 20.
		notice that gas price does not exceed 20 as defined by max fee.
	*/

	gasFeeCap, gasTipCap, err := t.suggestedFeeAndTip(ctx, request.GasPrice, boostPercent)
	if err != nil {
		return nil, err
	}

	return types.NewTx(&types.DynamicFeeTx{
		Nonce:     nonce,
		ChainID:   t.chainID,
		To:        request.To,
		Value:     request.Value,
		Gas:       gasLimit,
		GasFeeCap: gasFeeCap,
		GasTipCap: gasTipCap,
		Data:      request.Data,
	}), nil
}

// updateNetworkMetrics fetches and updates network congestion metrics
func (t *transactionService) updateNetworkMetrics(ctx context.Context) error {
	t.networkMetrics.mutex.Lock()
	defer t.networkMetrics.mutex.Unlock()

	// Skip update if recently updated (within last 30 seconds)
	if time.Since(t.networkMetrics.lastUpdate) < 30*time.Second {
		return nil
	}

	// Get current block number
	blockNumber, err := t.backend.BlockNumber(ctx)
	if err != nil {
		return err
	}

	// Get recent block headers to analyze congestion
	var totalGasUsed, totalGasLimit uint64
	blockCount := int64(MaxGasFeeHistoryBlocks)
	if blockCount > int64(blockNumber) {
		blockCount = int64(blockNumber)
	}

	for i := int64(0); i < blockCount; i++ {
		header, err := t.backend.HeaderByNumber(ctx, big.NewInt(int64(blockNumber)-i))
		if err != nil {
			continue
		}
		totalGasUsed += header.GasUsed
		totalGasLimit += header.GasLimit
	}

	if totalGasLimit > 0 {
		t.networkMetrics.congestionRatio = float64(totalGasUsed) / float64(totalGasLimit)
	}

	// Update base fee from latest block
	if blockCount > 0 {
		latestHeader, err := t.backend.HeaderByNumber(ctx, big.NewInt(int64(blockNumber)))
		if err == nil && latestHeader.BaseFee != nil {
			t.networkMetrics.avgBaseFee = new(big.Int).Set(latestHeader.BaseFee)
		}
	}

	t.networkMetrics.lastUpdate = time.Now()
	return nil
}

// calculateDynamicFees implements sophisticated fee calculation based on network conditions
func (t *transactionService) calculateDynamicFees(ctx context.Context, strategy GasFeeStrategy, boostPercent int) (*big.Int, *big.Int, error) {
	// Update network metrics
	if err := t.updateNetworkMetrics(ctx); err != nil {
		t.logger.Debug("failed to update network metrics", "error", err)
	}

	t.networkMetrics.mutex.RLock()
	congestionRatio := t.networkMetrics.congestionRatio
	avgBaseFee := new(big.Int).Set(t.networkMetrics.avgBaseFee)
	t.networkMetrics.mutex.RUnlock()

	// Get base suggestions from the backend
	suggestedGasPrice, err := t.backend.SuggestGasPrice(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get suggested gas price: %w", err)
	}

	suggestedTipCap, err := t.backend.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get suggested tip cap: %w", err)
	}

	// Apply strategy-specific adjustments
	var gasFeeCap, gasTipCap *big.Int

	switch strategy {
	case StrategyConservative:
		gasTipCap = new(big.Int).Div(new(big.Int).Mul(suggestedTipCap, big.NewInt(80)), big.NewInt(100)) // 80% of suggested tip
		gasFeeCap = new(big.Int).Add(avgBaseFee, gasTipCap)

	case StrategyStandard:
		gasTipCap = new(big.Int).Set(suggestedTipCap)
		gasFeeCap = new(big.Int).Set(suggestedGasPrice)

	case StrategyAggressive:
		gasTipCap = new(big.Int).Div(new(big.Int).Mul(suggestedTipCap, big.NewInt(150)), big.NewInt(100)) // 150% of suggested tip
		gasFeeCap = new(big.Int).Div(new(big.Int).Mul(suggestedGasPrice, big.NewInt(130)), big.NewInt(100)) // 130% of suggested price

	case StrategyDynamic:
		// Dynamic strategy adapts to network congestion
		congestionMultiplier := 1.0
		if congestionRatio > NetworkCongestionThreshold {
			congestionMultiplier = CongestionMultiplier
			t.logger.Debug("high network congestion detected", "ratio", congestionRatio, "multiplier", congestionMultiplier)
		}

		// Apply congestion-aware multiplier
		multiplierInt := big.NewInt(int64(congestionMultiplier * 100))
		gasTipCap = new(big.Int).Div(new(big.Int).Mul(suggestedTipCap, multiplierInt), big.NewInt(100))
		gasFeeCap = new(big.Int).Div(new(big.Int).Mul(suggestedGasPrice, multiplierInt), big.NewInt(100))

		// Ensure we have reasonable bounds
		minTip := big.NewInt(MinGasTipCap)
		maxTip := big.NewInt(MaxGasTipCap)
		if gasTipCap.Cmp(minTip) < 0 {
			gasTipCap = minTip
		}
		if gasTipCap.Cmp(maxTip) > 0 {
			gasTipCap = maxTip
		}
	}

	// Apply additional boost percentage if specified
	if boostPercent > 0 {
		boostMultiplier := big.NewInt(int64(boostPercent) + 100)
		gasTipCap = new(big.Int).Div(new(big.Int).Mul(gasTipCap, boostMultiplier), big.NewInt(100))
		gasFeeCap = new(big.Int).Div(new(big.Int).Mul(gasFeeCap, boostMultiplier), big.NewInt(100))
	}

	// Ensure gasFeeCap covers baseFee + tip
	minFeeCap := new(big.Int).Add(avgBaseFee, gasTipCap)
	if gasFeeCap.Cmp(minFeeCap) < 0 {
		gasFeeCap = minFeeCap
	}

	t.logger.Debug("calculated dynamic fees",
		"strategy", strategy,
		"congestion_ratio", congestionRatio,
		"gas_fee_cap", gasFeeCap,
		"gas_tip_cap", gasTipCap,
		"boost_percent", boostPercent)

	return gasFeeCap, gasTipCap, nil
}

func (t *transactionService) suggestedFeeAndTip(ctx context.Context, gasPrice *big.Int, boostPercent int) (*big.Int, *big.Int, error) {
	// If in legacy mode or gasPrice is explicitly provided, use legacy calculation
	if t.legacyMode || gasPrice != nil {
		var err error
		if gasPrice == nil {
			gasPrice, err = t.backend.SuggestGasPrice(ctx)
			if err != nil {
				return nil, nil, err
			}
			gasPrice = new(big.Int).Div(new(big.Int).Mul(big.NewInt(int64(boostPercent)+100), gasPrice), big.NewInt(100))
		}

		gasTipCap, err := t.backend.SuggestGasTipCap(ctx)
		if err != nil {
			return nil, nil, err
		}

		gasTipCap = new(big.Int).Div(new(big.Int).Mul(big.NewInt(int64(boostPercent)+100), gasTipCap), big.NewInt(100))
		gasFeeCap := new(big.Int).Add(gasTipCap, gasPrice)

		t.logger.Debug("using legacy fee calculation", "gas_price", gasPrice, "gas_max_fee", gasFeeCap, "gas_max_tip", gasTipCap)
		return gasFeeCap, gasTipCap, nil
	}

	// Use enhanced dynamic fee calculation
	return t.calculateDynamicFees(ctx, t.feeStrategy, boostPercent)
}

// calculateBlobFee calculates blob fee for Pectra upgrade support
// This prepares for EIP-4844 blob transactions that will be enhanced in Pectra
func (t *transactionService) calculateBlobFee(ctx context.Context, blobCount int) (*big.Int, error) {
	if blobCount <= 0 {
		return big.NewInt(0), nil
	}

	t.networkMetrics.mutex.RLock()
	blobBaseFee := new(big.Int).Set(t.networkMetrics.blobBaseFee)
	t.networkMetrics.mutex.RUnlock()

	// In Pectra, blob capacity increases from 3 to 6 blobs per block
	// This affects the blob fee calculation
	blobFee := new(big.Int).Mul(blobBaseFee, big.NewInt(int64(blobCount)))
	
	t.logger.Debug("calculated blob fee", 
		"blob_count", blobCount, 
		"blob_base_fee", blobBaseFee, 
		"total_blob_fee", blobFee)
	
	return blobFee, nil
}

// SetFeeStrategy allows changing the gas fee calculation strategy
func (t *transactionService) SetFeeStrategy(strategy GasFeeStrategy) {
	t.feeStrategy = strategy
	t.logger.Debug("gas fee strategy updated", "strategy", strategy)
}

// GetNetworkMetrics returns current network congestion metrics
func (t *transactionService) GetNetworkMetrics() (congestion float64, avgBaseFee *big.Int, avgTipFee *big.Int) {
	t.networkMetrics.mutex.RLock()
	defer t.networkMetrics.mutex.RUnlock()
	
	return t.networkMetrics.congestionRatio, 
		   new(big.Int).Set(t.networkMetrics.avgBaseFee),
		   new(big.Int).Set(t.networkMetrics.avgPriorityFee)
}

// PredictOptimalGasTime predicts the optimal time to send a transaction based on network conditions
func (t *transactionService) PredictOptimalGasTime(ctx context.Context) (time.Duration, error) {
	if err := t.updateNetworkMetrics(ctx); err != nil {
		return 0, err
	}

	t.networkMetrics.mutex.RLock()
	congestionRatio := t.networkMetrics.congestionRatio
	t.networkMetrics.mutex.RUnlock()

	// If network is highly congested, suggest waiting
	if congestionRatio > 0.9 {
		return 5 * time.Minute, nil // Suggest waiting 5 minutes
	} else if congestionRatio > NetworkCongestionThreshold {
		return 2 * time.Minute, nil // Suggest waiting 2 minutes
	}

	return 0, nil // Send immediately
}

// CalculateBlobFee calculates blob fee for Pectra upgrade support
func (t *transactionService) CalculateBlobFee(ctx context.Context, blobCount int) (*big.Int, error) {
	return t.calculateBlobFee(ctx, blobCount)
}

// SetLegacyMode enables/disables legacy gas calculation for backward compatibility
func (t *transactionService) SetLegacyMode(enabled bool) {
	t.legacyMode = enabled
	if enabled {
		t.logger.Debug("legacy gas calculation mode enabled")
	} else {
		t.logger.Debug("enhanced gas calculation mode enabled")
	}
}

func storedTransactionKey(txHash common.Hash) string {
	return fmt.Sprintf("%s%x", storedTransactionPrefix, txHash)
}

func pendingTransactionKey(txHash common.Hash) string {
	return fmt.Sprintf("%s%x", pendingTransactionPrefix, txHash)
}

func (t *transactionService) nextNonce(ctx context.Context) (uint64, error) {
	onchainNonce, err := t.backend.PendingNonceAt(ctx, t.sender)
	if err != nil {
		return 0, err
	}

	pendingTxs, err := t.PendingTransactions()
	if err != nil {
		return 0, err
	}

	pendingTxs = t.filterPendingTransactions(t.ctx, pendingTxs)

	// PendingNonceAt returns the nonce we should use, but we will
	// compare this to our pending tx list, therefore the -1.
	var maxNonce uint64 = onchainNonce - 1
	for _, txHash := range pendingTxs {
		trx, _, err := t.backend.TransactionByHash(ctx, txHash)

		if err != nil {
			t.logger.Error(err, "pending transaction not found", "tx", txHash)
			return 0, err
		}

		maxNonce = max(maxNonce, trx.Nonce())
	}

	return maxNonce + 1, nil
}

// WaitForReceipt waits until either the transaction with the given hash has
// been mined or the context is cancelled.
func (t *transactionService) WaitForReceipt(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
	receiptC, errC, err := t.WatchSentTransaction(txHash)
	if err != nil {
		return nil, err
	}
	select {
	case receipt := <-receiptC:
		return &receipt, nil
	case err := <-errC:
		return nil, err
	// don't wait longer than the context that was passed in
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *transactionService) WatchSentTransaction(txHash common.Hash) (<-chan types.Receipt, <-chan error, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	// loading the tx here guarantees it was in fact sent from this transaction service
	// also it allows us to avoid having to load the transaction during the watch loop
	storedTransaction, err := t.StoredTransaction(txHash)
	if err != nil {
		return nil, nil, err
	}

	return t.monitor.WatchTransaction(txHash, storedTransaction.Nonce)
}

func (t *transactionService) PendingTransactions() ([]common.Hash, error) {
	var txHashes []common.Hash = make([]common.Hash, 0)
	err := t.store.Iterate(pendingTransactionPrefix, func(key, value []byte) (stop bool, err error) {
		txHash := common.HexToHash(strings.TrimPrefix(string(key), pendingTransactionPrefix))
		txHashes = append(txHashes, txHash)
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return txHashes, nil
}

// filterPendingTransactions will filter supplied transaction hashes removing those that are not pending anymore.
// Removed transactions will be also removed from store.
func (t *transactionService) filterPendingTransactions(ctx context.Context, txHashes []common.Hash) []common.Hash {
	result := make([]common.Hash, 0, len(txHashes))

	for _, txHash := range txHashes {
		_, isPending, err := t.backend.TransactionByHash(ctx, txHash)

		// When error occurres consider transaction as pending (so this transaction won't be filtered out),
		// unless it was not found
		if err != nil {
			if errors.Is(err, ethereum.NotFound) {
				t.logger.Error(err, "pending transactions not found", "tx", txHash)

				isPending = false
			} else {
				isPending = true
			}
		}

		if isPending {
			result = append(result, txHash)
		} else {
			err := t.store.Delete(pendingTransactionKey(txHash))
			if err != nil {
				t.logger.Error(err, "error while unregistering transaction as pending", "tx", txHash)
			}
		}
	}

	return result
}

func (t *transactionService) ResendTransaction(ctx context.Context, txHash common.Hash) error {
	storedTransaction, err := t.StoredTransaction(txHash)
	if err != nil {
		return err
	}

	gasFeeCap, gasTipCap, err := t.suggestedFeeAndTip(ctx, sctx.GetGasPrice(ctx), storedTransaction.GasTipBoost)
	if err != nil {
		return err
	}

	tx := types.NewTx(&types.DynamicFeeTx{
		Nonce:     storedTransaction.Nonce,
		ChainID:   t.chainID,
		To:        storedTransaction.To,
		Value:     storedTransaction.Value,
		Gas:       storedTransaction.GasLimit,
		GasTipCap: gasTipCap,
		GasFeeCap: gasFeeCap,
		Data:      storedTransaction.Data,
	})

	signedTx, err := t.signer.SignTx(tx, t.chainID)
	if err != nil {
		return err
	}

	if signedTx.Hash() != txHash {
		return errors.New("transaction hash changed")
	}

	err = t.backend.SendTransaction(t.ctx, signedTx)
	if err != nil {
		if strings.Contains(err.Error(), "already imported") {
			return ErrAlreadyImported
		}
	}
	return nil
}

func (t *transactionService) CancelTransaction(ctx context.Context, originalTxHash common.Hash) (common.Hash, error) {
	storedTransaction, err := t.StoredTransaction(originalTxHash)
	if err != nil {
		return common.Hash{}, err
	}

	gasFeeCap, gasTipCap, err := t.suggestedFeeAndTip(ctx, sctx.GetGasPrice(ctx), 0)
	if err != nil {
		return common.Hash{}, err
	}

	if gasFeeCap.Cmp(storedTransaction.GasFeeCap) <= 0 {
		gasFeeCap = storedTransaction.GasFeeCap
	}

	if gasTipCap.Cmp(storedTransaction.GasTipCap) <= 0 {
		gasTipCap = storedTransaction.GasTipCap
	}

	gasTipCap = new(big.Int).Div(new(big.Int).Mul(big.NewInt(int64(10)+100), gasTipCap), big.NewInt(100))

	gasFeeCap.Add(gasFeeCap, gasTipCap)

	signedTx, err := t.signer.SignTx(types.NewTx(&types.DynamicFeeTx{
		Nonce:     storedTransaction.Nonce,
		ChainID:   t.chainID,
		To:        &t.sender,
		Value:     big.NewInt(0),
		Gas:       21000,
		GasTipCap: gasTipCap,
		GasFeeCap: gasFeeCap,
		Data:      []byte{},
	}), t.chainID)
	if err != nil {
		return common.Hash{}, err
	}

	err = t.backend.SendTransaction(t.ctx, signedTx)
	if err != nil {
		return common.Hash{}, err
	}

	txHash := signedTx.Hash()
	err = t.store.Put(storedTransactionKey(txHash), StoredTransaction{
		To:          signedTx.To(),
		Data:        signedTx.Data(),
		GasPrice:    signedTx.GasPrice(),
		GasLimit:    signedTx.Gas(),
		GasFeeCap:   signedTx.GasFeeCap(),
		GasTipBoost: storedTransaction.GasTipBoost,
		GasTipCap:   signedTx.GasTipCap(),
		Value:       signedTx.Value(),
		Nonce:       signedTx.Nonce(),
		Created:     time.Now().Unix(),
		Description: fmt.Sprintf("%s (cancellation)", storedTransaction.Description),
	})
	if err != nil {
		return common.Hash{}, err
	}

	err = t.store.Put(pendingTransactionKey(txHash), struct{}{})
	if err != nil {
		return common.Hash{}, err
	}

	t.waitForPendingTx(txHash)

	return txHash, err
}

func (t *transactionService) Close() error {
	t.cancel()
	t.wg.Wait()
	return nil
}

func (t *transactionService) TransactionFee(ctx context.Context, txHash common.Hash) (*big.Int, error) {
	trx, _, err := t.backend.TransactionByHash(ctx, txHash)
	if err != nil {
		return nil, err
	}
	return trx.Cost(), nil
}

func (t *transactionService) UnwrapABIError(ctx context.Context, req *TxRequest, err error, abiErrors map[string]abi.Error) error {
	if err == nil {
		return nil
	}

	_, cErr := t.Call(ctx, req)
	if cErr == nil {
		return err
	}
	err = fmt.Errorf("%w: %s", err, cErr) //nolint:errorlint

	var derr rpc.DataError
	if !errors.As(cErr, &derr) {
		return err
	}

	res, ok := derr.ErrorData().(string)
	if !ok {
		return err
	}
	buf := common.FromHex(res)

	if reason, uErr := abi.UnpackRevert(buf); uErr == nil {
		return fmt.Errorf("%w: %s", err, reason)
	}

	for _, abiError := range abiErrors {
		if !bytes.Equal(buf[:4], abiError.ID[:4]) {
			continue
		}

		data, uErr := abiError.Unpack(buf)
		if uErr != nil {
			continue
		}

		values, ok := data.([]interface{})
		if !ok {
			values = make([]interface{}, len(abiError.Inputs))
			for i := range values {
				values[i] = "?"
			}
		}

		params := make([]string, len(abiError.Inputs))
		for i, input := range abiError.Inputs {
			if input.Name == "" {
				input.Name = fmt.Sprintf("arg%d", i)
			}
			params[i] = fmt.Sprintf("%s=%v", input.Name, values[i])

		}

		return fmt.Errorf("%w: %s(%s)", err, abiError.Name, strings.Join(params, ","))
	}

	return err
}
