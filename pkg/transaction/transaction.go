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
	"github.com/ethersphere/bee/v2/pkg/safe"
	"github.com/ethersphere/bee/v2/pkg/sctx"
	"github.com/ethersphere/bee/v2/pkg/storage"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "transaction"

const (
	storedTransactionPrefix  = "transaction_stored_"
	pendingTransactionPrefix = "transaction_pending_"
)

var (
	// ErrTransactionReverted denotes that the sent transaction has been
	// reverted.
	ErrTransactionReverted = errors.New("transaction reverted")
	ErrUnknownTransaction  = errors.New("unknown transaction")
	ErrAlreadyImported     = errors.New("already imported")
	// ErrReplacementUnderpriced denotes that the node kept the previous
	// transaction with the same nonce because the new fees are too low.
	ErrReplacementUnderpriced = errors.New("replacement transaction underpriced")
	// ErrNonceUsed denotes that the nonce of the transaction is already used on chain.
	ErrNonceUsed = errors.New("nonce already used")
	// ErrSuperseded denotes that a newer transaction has taken the nonce.
	// One hash of the old chain may still be mined, but the old chain can no longer be resent or cancelled.
	ErrSuperseded = errors.New("a newer transaction uses this nonce")
	// ErrCancellation denotes that the hash is a cancellation of its nonce.
	ErrCancellation = errors.New("transaction is a cancellation")
	// ErrNotInResendChain denotes that the hash is not the transaction this nonce is being resent as.
	ErrNotInResendChain = errors.New("transaction is not the current transaction for its nonce")
	// ErrMissingFees denotes that a stored transaction has no fees to bump.
	ErrMissingFees = errors.New("stored transaction has no fees to bump")
	// ErrReceiptTimeout means the transaction was broadcast, but no receipt
	// arrived before the wait timed out. The transaction may still be mined.
	ErrReceiptTimeout = errors.New("transaction was not mined before the wait timed out; it may still be pending")
	// ErrApprovalTimeout means the token approval before an operation was not
	// mined in time. The approval hash is omitted: resending it does not
	// complete the operation and only spends gas.
	ErrApprovalTimeout = errors.New("approval was not mined before the wait timed out")
)

// ReceiptTimeoutError is returned by SendAndWait when the receipt wait times out.
// Hash is the transaction the client can resend. It is zero when resending would not complete the operation.
type ReceiptTimeoutError struct {
	Hash common.Hash
	err  error
}

// NewReceiptTimeout reports that hash was broadcast but not mined in time.
func NewReceiptTimeout(hash common.Hash) *ReceiptTimeoutError {
	return &ReceiptTimeoutError{Hash: hash, err: ErrReceiptTimeout}
}

// NewApprovalTimeout reports that the token approval was not mined in time.
// The approval hash is omitted.
func NewApprovalTimeout() *ReceiptTimeoutError {
	return &ReceiptTimeoutError{err: ErrApprovalTimeout}
}

func (e *ReceiptTimeoutError) Error() string {
	if e.Hash == (common.Hash{}) {
		return e.cause().Error()
	}
	return fmt.Sprintf("%s: %s", e.cause(), e.Hash.Hex())
}

func (e *ReceiptTimeoutError) Unwrap() error {
	return e.cause()
}

// Message is the client-facing text, without the transaction hash.
func (e *ReceiptTimeoutError) Message() string {
	return e.cause().Error()
}

// TxHash is the hex hash to show the client, or empty when there is nothing to resend.
func (e *ReceiptTimeoutError) TxHash() string {
	if e.Hash == (common.Hash{}) {
		return ""
	}
	return e.Hash.String()
}

func (e *ReceiptTimeoutError) cause() error {
	if e.err != nil {
		return e.err
	}
	return ErrReceiptTimeout
}

const (
	// cancellationDescriptionSuffix marks a stored cancellation. CancelTransaction appends it.
	cancellationDescriptionSuffix = " (cancellation)"

	DefaultGasLimit           = 1_000_000 // Used for contract operations when setGasLimit flag is enabled
	MinReplacementBumpPercent = 15
	DefaultTipBoostPercent    = 25
	MaxGasLimit               = 10_000_000 // Maximum allowed gas limit to prevent excessive values
	MinGasLimit               = 21_000     // Minimum gas for any transaction
	GasBufferPercent          = 33         // Add 33% buffer to estimated gas
	FallbackGasLimit          = 900_000    // Fallback when estimation fails and no minimum is set
)

// TxRequest describes a request for a transaction that can be executed.
type TxRequest struct {
	To                   *common.Address // recipient of the transaction
	Data                 []byte          // transaction data
	GasPrice             *big.Int        // gas price or nil if suggested gas price should be used
	GasLimit             uint64          // gas limit or 0 if it should be estimated
	MinEstimatedGasLimit uint64          // minimum gas limit to use if the gas limit was estimated; it will not apply when this value is 0 or when GasLimit is not 0
	GasFeeCap            *big.Int        // adds a cap to maximum fee user is willing to pay
	FeePriority          int             // fee priority; when set, it takes precedence over GasPrice
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
	FeePriority int             // fee priority that priced the transaction
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
	// SendAndWait sends the request and waits until the transaction is mined.
	// It returns ErrTransactionReverted when the transaction failed.
	// timeout limits the wait for the receipt. Zero waits until the transaction
	// is mined or ctx is cancelled. A deadline returns *ReceiptTimeoutError with
	// the broadcast hash. Broadcasting the transaction is not limited by timeout.
	SendAndWait(ctx context.Context, request *TxRequest, tipCapBoostPercent int, timeout time.Duration) (receipt *types.Receipt, err error)
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
	// ResendTransaction broadcasts a previously sent transaction again with the same nonce and content and new fees.
	ResendTransaction(ctx context.Context, txHash common.Hash, bumpPercent int) (*ResendResult, error)
	// KnownTransaction returns the newest hash of txHash's chain that the node has answered for.
	KnownTransaction(ctx context.Context, txHash common.Hash) (common.Hash, bool, error)
	// TransactionReceipt returns the receipt of the transaction.
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	// CancelTransaction cancels a previously sent transaction by double-spending its nonce with zero-transfer one.
	CancelTransaction(ctx context.Context, originalTxHash common.Hash) (common.Hash, error)
	// TransactionFee retrieves the transaction fee
	TransactionFee(ctx context.Context, txHash common.Hash) (*big.Int, error)
	// UnwrapABIError tries to unwrap the ABI error if the given error is not nil.
	// The original error is wrapped together with the ABI error if it exists.
	UnwrapABIError(ctx context.Context, req *TxRequest, err error, abiErrors map[string]abi.Error) error
}

type transactionService struct {
	wg     sync.WaitGroup
	lock   sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc

	logger           log.Logger
	backend          Backend
	signer           crypto.Signer
	sender           common.Address
	store            storage.StateStorer
	chainID          *big.Int
	monitor          Monitor
	fallbackGasLimit uint64
}

// NewService creates a new transaction service.
func NewService(logger log.Logger, overlayEthAddress common.Address, backend Backend, signer crypto.Signer, store storage.StateStorer, chainID *big.Int, monitor Monitor, fallbackGasLimit uint64) (Service, error) {
	senderAddress, err := signer.EthereumAddress()
	if err != nil {
		return nil, err
	}

	if fallbackGasLimit == 0 {
		fallbackGasLimit = FallbackGasLimit
	}

	ctx, cancel := context.WithCancel(context.Background())

	t := &transactionService{
		ctx:              ctx,
		cancel:           cancel,
		logger:           logger.WithName(loggerName).WithValues("sender_address", overlayEthAddress).Register(),
		backend:          backend,
		signer:           signer,
		sender:           senderAddress,
		store:            store,
		chainID:          chainID,
		monitor:          monitor,
		fallbackGasLimit: fallbackGasLimit,
	}

	if err = t.waitForAllPendingTx(); err != nil {
		return nil, err
	}

	return t, nil
}

func (t *transactionService) waitForAllPendingTx() error {
	pendingTxs, err := t.PendingTransactions()
	if err != nil {
		return err
	}

	pending := t.filterPendingTransactions(t.ctx, pendingTxs)

	for txHash := range pending {
		t.waitForPendingTx(txHash)
	}

	return nil
}

// Send creates a transaction based on the request (with gasprice increased by the provided percentage) and sends it.
// A fatal broadcast error is returned and nothing is stored.
// Any other broadcast error is stored and returned as success: the transaction may already be in the mempool.
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
	if !txMayBeInMempool(err) {
		return common.Hash{}, err
	}
	if err != nil {
		t.logger.Warning("transaction broadcast returned an error; the transaction may be in the mempool",
			"tx", signedTx.Hash(),
			"nonce", signedTx.Nonce(),
			"error", err,
		)
	}

	txHash = signedTx.Hash()

	// A fee priority prices the transaction without the tip boost.
	gasTipBoost := boostPercent
	if request.FeePriority != 0 {
		gasTipBoost = 0
	}

	err = t.store.Put(storedTransactionKey(txHash), StoredTransaction{
		To:          signedTx.To(),
		Data:        signedTx.Data(),
		GasPrice:    signedTx.GasPrice(),
		GasLimit:    signedTx.Gas(),
		GasTipBoost: gasTipBoost,
		GasTipCap:   signedTx.GasTipCap(),
		GasFeeCap:   signedTx.GasFeeCap(),
		Value:       signedTx.Value(),
		Nonce:       signedTx.Nonce(),
		Created:     time.Now().Unix(),
		Description: request.Description,
		FeePriority: request.FeePriority,
	})
	if err != nil {
		return common.Hash{}, err
	}

	err = t.store.Put(pendingTransactionKey(txHash), struct{}{})
	if err != nil {
		return common.Hash{}, err
	}

	// The nonce now belongs to this transaction. A resend chain left for it belongs to
	// dropped transactions, and resending one of them must not replace this one.
	// That chain is kept as superseded, because one of its hashes may still be mined.
	owner := &ResentTransaction{Nonce: nonce, CurrentHash: txHash}
	switch prev, err := t.getResendChain(nonce); {
	case err == nil:
		owner.Superseded = append(prev.Superseded, prev.chainHashes())
	case !errors.Is(err, storage.ErrNotFound):
		t.logger.Warning("loading nonce owner failed", "nonce", nonce, "error", err)
	}
	if err := t.store.Put(resendTxKey(nonce), owner); err != nil {
		t.logger.Warning("storing nonce owner failed", "nonce", nonce, "tx", txHash, "error", err)
	}

	t.waitForPendingTx(txHash)

	return signedTx.Hash(), nil
}

// SendAndWait sends the request and waits for the transaction to be mined.
// A transaction that ran out of an estimated gas limit is sent once more with
// twice that limit, capped at MaxGasLimit; there is no retry when the cap
// would not raise the limit. A gas limit set by the caller is never changed.
// timeout limits each wait for a receipt. Zero waits until the receipt arrives
// or ctx is cancelled. A deadline returns *ReceiptTimeoutError with the
// broadcast hash. Each attempt, including the out-of-gas retry, gets the
// full timeout. Broadcasting is not limited by timeout.
func (t *transactionService) SendAndWait(ctx context.Context, request *TxRequest, boostPercent int, timeout time.Duration) (*types.Receipt, error) {
	receipt, txHash, err := t.sendAndWait(ctx, request, boostPercent, timeout)
	if err != nil {
		return nil, err
	}
	if receipt.Status == types.ReceiptStatusSuccessful {
		return receipt, nil
	}

	if request.GasLimit != 0 {
		return nil, ErrTransactionReverted
	}
	gasLimit, err := t.outOfGasLimit(txHash, receipt)
	if err != nil {
		return nil, err
	}

	retryGasLimit := min(2*gasLimit, MaxGasLimit)
	if retryGasLimit <= gasLimit {
		return nil, ErrTransactionReverted
	}

	retry := *request
	retry.GasLimit = retryGasLimit
	retry.Description = request.Description + " (retry after out of gas)"

	t.logger.Warning("transaction ran out of gas, sending it again with a higher gas limit",
		"tx_hash", txHash,
		"gas_limit", gasLimit,
		"retry_gas_limit", retry.GasLimit,
		"description", request.Description,
	)

	receipt, _, err = t.sendAndWait(ctx, &retry, boostPercent, timeout)
	if err != nil {
		return nil, fmt.Errorf("retry after out of gas: %w", err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return nil, fmt.Errorf("retry after out of gas: %w", ErrTransactionReverted)
	}

	return receipt, nil
}

// sendAndWait sends the request and returns the receipt whatever its status.
// A positive timeout limits only the wait for the receipt.
func (t *transactionService) sendAndWait(ctx context.Context, request *TxRequest, boostPercent int, timeout time.Duration) (*types.Receipt, common.Hash, error) {
	txHash, err := t.Send(ctx, request, boostPercent)
	if err != nil {
		return nil, common.Hash{}, err
	}

	waitCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	receipt, err := t.WaitForReceipt(waitCtx, txHash)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, txHash, NewReceiptTimeout(txHash)
		}
		return nil, common.Hash{}, err
	}

	return receipt, txHash, nil
}

// outOfGasLimit returns the gas limit of a failed transaction that used all
// of it (a revert refunds unused gas), or ErrTransactionReverted otherwise.
func (t *transactionService) outOfGasLimit(txHash common.Hash, receipt *types.Receipt) (uint64, error) {
	stored, err := t.StoredTransaction(txHash)
	if err != nil {
		return 0, fmt.Errorf("%w: stored transaction: %w", ErrTransactionReverted, err)
	}
	if stored.GasLimit == 0 || receipt.GasUsed < stored.GasLimit {
		return 0, ErrTransactionReverted
	}
	return stored.GasLimit, nil
}

func (t *transactionService) waitForPendingTx(txHash common.Hash) {
	t.wg.Go(func() {
		safe.Run(t.logger, "transaction-wait-pending", func() {
			switch _, err := t.WaitForReceipt(t.ctx, txHash); err {
			case nil:
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
		})
	})
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
		// Estimate gas using pending state for consistency with PendingNonceAt
		gasLimit, err = t.backend.EstimateGas(ctx, ethereum.CallMsg{
			From:  t.sender,
			To:    request.To,
			Data:  request.Data,
			Value: request.Value,
		})

		if err != nil {
			t.logger.Warning("gas estimation failed, using fallback",
				"error", err,
				"description", request.Description,
			)

			if request.MinEstimatedGasLimit > 0 {
				gasLimit = request.MinEstimatedGasLimit
			} else if len(request.Data) > 0 {
				// Contract call - use configured fallback
				gasLimit = t.fallbackGasLimit
			} else {
				// Simple transfer - use minimum
				gasLimit = MinGasLimit
			}
		} else {
			// Estimation succeeded - add buffer for state changes
			gasLimit += gasLimit * GasBufferPercent / 100

			// Apply minimum if specified
			if gasLimit < request.MinEstimatedGasLimit {
				gasLimit = request.MinEstimatedGasLimit
			}

			// Cap at maximum
			if gasLimit > MaxGasLimit {
				gasLimit = MaxGasLimit
			}
		}

		// Ensure absolute minimum
		if gasLimit < MinGasLimit {
			gasLimit = MinGasLimit
		}
	} else {
		// Use provided gas limit with bounds validation
		gasLimit = min(max(request.GasLimit, MinGasLimit), MaxGasLimit)
	}

	if gasLimit == 0 {
		return nil, errors.New("gas limit cannot be zero")
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

	var gasFeeCap, gasTipCap *big.Int
	if request.FeePriority != 0 {
		gasFeeCap, gasTipCap, err = t.backend.SuggestFeeHistory(ctx, request.FeePriority)
	} else {
		gasFeeCap, gasTipCap, err = t.backend.SuggestedFeeAndTip(ctx, request.GasPrice, boostPercent)
	}
	if err != nil {
		return nil, err
	}

	t.logger.Debug("prepared transaction",
		"to", request.To,
		"value", request.Value,
		"gas_limit", gasLimit,
		"gas_fee_cap", gasFeeCap,
		"gas_tip_cap", gasTipCap,
		"fee_priority", FeePriorityName(request.FeePriority),
		"nonce", nonce,
		"description", request.Description,
	)

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

	pending := t.filterPendingTransactions(t.ctx, pendingTxs)

	// PendingNonceAt returns the nonce we should use, but we will
	// compare this to our pending tx list, therefore the -1.
	maxNonce := onchainNonce - 1
	for txHash, trx := range pending {
		if trx == nil {
			t.logger.Warning("pending transaction data unavailable, relying on onchain nonce", "tx", txHash)
			continue
		}
		maxNonce = max(maxNonce, trx.Nonce())
	}

	return maxNonce + 1, nil
}

func (t *transactionService) WaitForReceipt(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
	receiptC, errC, err := t.WatchSentTransaction(txHash)
	if err != nil {
		return nil, err
	}
	select {
	case receipt := <-receiptC:
		return &receipt, nil
	case err := <-errC:
		if errors.Is(err, ErrTransactionCancelled) {
			receipt, rerr := t.TransactionReceipt(ctx, txHash)
			if rerr == nil {
				return receipt, nil
			}
			if !errors.Is(rerr, ethereum.NotFound) {
				return nil, fmt.Errorf("%w: %w", err, rerr)
			}
		}
		return nil, err
	// don't wait longer than the context that was passed in
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// WatchSentTransaction starts watching the given transaction.
// This wraps the monitor's watch function by loading the correct nonce from the store.
// This is only valid for a transaction sent by this service.
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
	txHashes := make([]common.Hash, 0)
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
// Returns the pending transactions keyed by hash.
func (t *transactionService) filterPendingTransactions(ctx context.Context, txHashes []common.Hash) map[common.Hash]*types.Transaction {
	result := make(map[common.Hash]*types.Transaction, len(txHashes))

	for _, txHash := range txHashes {
		trx, isPending, err := t.backend.TransactionByHash(ctx, txHash)
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
			result[txHash] = trx
		} else {
			err := t.store.Delete(pendingTransactionKey(txHash))
			if err != nil {
				t.logger.Error(err, "error while unregistering transaction as pending", "tx", txHash)
			}
		}
	}

	return result
}

// CancelTransaction cancels a previously sent transaction by double-spending its nonce with a zero-value transfer.
// When the nonce has a resend chain, the cancellation outbids the current hash of that chain.
// The cancellation itself is not added to the chain.
func (t *transactionService) CancelTransaction(ctx context.Context, originalTxHash common.Hash) (common.Hash, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	storedTransaction, err := t.StoredTransaction(originalTxHash)
	if err != nil {
		return common.Hash{}, err
	}

	chain, err := t.getResendChain(storedTransaction.Nonce)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		// The nonce has no owner record.
	case err != nil:
		return common.Hash{}, err
	case !chain.inChain(originalTxHash):
		return common.Hash{}, outsideChainError(chain, storedTransaction, originalTxHash)
	case chain.CurrentHash != originalTxHash:
		// The fees of the cancellation must outbid the latest resend, not an old hash.
		storedTransaction, err = t.StoredTransaction(chain.CurrentHash)
		if err != nil {
			return common.Hash{}, fmt.Errorf("current transaction %s: %w", chain.CurrentHash, err)
		}
	}

	marketFee, marketTip, err := t.backend.SuggestedFeeAndTip(ctx, sctx.GetGasPrice(ctx), DefaultTipBoostPercent)
	if err != nil {
		return common.Hash{}, err
	}

	gasFeeCap := maxFee(marketFee, raiseByPercent(storedTransaction.GasFeeCap, MinReplacementBumpPercent))
	gasTipCap := maxFee(marketTip, raiseByPercent(storedTransaction.GasTipCap, MinReplacementBumpPercent))

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

	sendCtx, cancel := context.WithTimeout(t.ctx, resendBroadcastTimeout)
	err = t.backend.SendTransaction(sendCtx, signedTx)
	cancel()
	if rerr := rejectedBroadcast(err); rerr != nil {
		return common.Hash{}, rerr
	}
	if err != nil {
		t.logger.Warning("transaction broadcast returned an error; the transaction may be in the mempool",
			"tx", signedTx.Hash(),
			"nonce", signedTx.Nonce(),
			"error", err,
		)
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
		Description: storedTransaction.Description + cancellationDescriptionSuffix,
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
	hashes, err := t.knownHashes(txHash)
	if err != nil {
		return nil, err
	}
	if len(hashes) > 1 {
		txHash, _, err = t.KnownTransaction(ctx, txHash)
		if err != nil {
			return nil, err
		}
	}
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

	// Revert data must be at least a 4-byte error selector; some RPC providers
	// return empty ("0x") or malformed data for a reverted transaction, which
	// would otherwise panic on the buf[:4] slice below.
	if len(buf) < 4 {
		return err
	}

	for _, abiError := range abiErrors {
		if !bytes.Equal(buf[:4], abiError.ID[:4]) {
			continue
		}

		data, uErr := abiError.Unpack(buf)
		if uErr != nil {
			continue
		}

		values, ok := data.([]any)
		if !ok {
			values = make([]any, len(abiError.Inputs))
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
