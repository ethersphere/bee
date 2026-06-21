// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsim

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/transaction"
)

var (
	// ErrNotImplemented is returned for backend methods that the simulator does not support.
	ErrNotImplemented = errors.New("chainsim: not implemented")
	// ErrClosed is returned when the simulator has been closed.
	ErrClosed = errors.New("chainsim: closed")
)

// SimChain simulates a minimal EIP-1559 chain for transaction retry testing.
type SimChain struct {
	mu     sync.RWMutex
	cfg    Config
	signer types.Signer

	blockNum uint64
	blockTs  uint64
	baseFee  *big.Int

	pool         *mempool
	blocks       []*simBlock
	nonces       map[common.Address]uint64
	nonceHistory map[common.Address][]nonceRecord
	balances     map[common.Address]*big.Int

	receipts   map[common.Hash]*receiptRecord
	minedTxs   map[common.Hash]*types.Transaction
	minedOrder []minedRef

	congestion          float64
	backgroundTipMean   *big.Int
	backgroundTipStdDev *big.Int
	minMempoolTip       *big.Int
	estimateGas         uint64

	revertAddresses map[common.Address]struct{}

	errInjections map[string][]errorInjection
	errMu         sync.Mutex
	rng           *rand.Rand

	ctx    context.Context
	cancel context.CancelFunc
	closed bool

	onBlockCommit func(blockNum uint64)

	scheduledBlockDelay time.Duration

	logger log.Logger
	stats  Stats
}

type simBlock struct {
	number   uint64
	time     uint64
	baseFee  *big.Int
	gasUsed  uint64
	gasLimit uint64
	tips     []*big.Int
	txHashes []common.Hash
}

type receiptRecord struct {
	receipt    *types.Receipt
	includedAt uint64
}

type nonceRecord struct {
	blockNum uint64
	nonce    uint64
}

type minedRef struct {
	block uint64
	hash  common.Hash
}

type errorInjection struct {
	err   error
	count int
}

// New creates a simulated chain with a genesis block.
func New(cfg Config) *SimChain {
	cfg = cfg.normalized()
	ctx, cancel := context.WithCancel(context.Background())

	s := &SimChain{
		cfg:                 cfg,
		signer:              types.LatestSignerForChainID(cfg.ChainID),
		blockNum:            0,
		blockTs:             uint64(time.Now().Unix()),
		baseFee:             new(big.Int).Set(cfg.InitialBaseFee),
		pool:                newMempool(cfg.MaxMempoolSize, cfg.MempoolTTL),
		blocks:              make([]*simBlock, 0, 16),
		nonces:              make(map[common.Address]uint64),
		nonceHistory:        make(map[common.Address][]nonceRecord),
		balances:            make(map[common.Address]*big.Int),
		receipts:            make(map[common.Hash]*receiptRecord),
		minedTxs:            make(map[common.Hash]*types.Transaction),
		congestion:          cfg.InitialCongestion,
		backgroundTipMean:   new(big.Int).Set(cfg.BackgroundTipMean),
		backgroundTipStdDev: new(big.Int).Set(cfg.BackgroundTipStdDev),
		minMempoolTip:       new(big.Int).Set(cfg.MinMempoolTip),
		estimateGas:         cfg.EstimateGas,
		revertAddresses:     make(map[common.Address]struct{}),
		errInjections:       make(map[string][]errorInjection),
		rng:                 rand.New(rand.NewSource(cfg.RNGSeed)), //nolint:gosec // deterministic simulation RNG
		ctx:                 ctx,
		cancel:              cancel,
		logger:              log.Noop,
		stats:               newStats(),
	}

	s.blocks = append(s.blocks, &simBlock{
		number:   0,
		time:     s.blockTs,
		baseFee:  new(big.Int).Set(s.baseFee),
		gasLimit: cfg.BlockGasLimit,
	})

	return s
}

var _ transaction.Backend = (*SimChain)(nil)

func (s *SimChain) confirmedNonce(sender common.Address) uint64 {
	return s.nonces[sender]
}

func (s *SimChain) recordNonce(sender common.Address, blockNum, nonce uint64) {
	s.nonces[sender] = nonce
	s.nonceHistory[sender] = append(s.nonceHistory[sender], nonceRecord{
		blockNum: blockNum,
		nonce:    nonce,
	})
}

func (s *SimChain) nonceAtBlock(sender common.Address, blockNum uint64) uint64 {
	history := s.nonceHistory[sender]
	var nonce uint64
	for _, rec := range history {
		if rec.blockNum > blockNum {
			break
		}
		nonce = rec.nonce
	}
	return nonce
}

// pruneNonceHistory keeps the last record strictly before cutoff plus all records
// at or after cutoff so nonceAtBlock still interpolates correctly.
func pruneNonceHistory(hist []nonceRecord, cutoff uint64) []nonceRecord {
	if len(hist) == 0 {
		return hist
	}
	keepFrom := 0
	for i, rec := range hist {
		if rec.blockNum < cutoff {
			keepFrom = i
		} else {
			break
		}
	}
	if keepFrom == 0 {
		return hist
	}
	return append([]nonceRecord(nil), hist[keepFrom:]...)
}

func (s *SimChain) trimHistoryLocked() {
	retention := s.cfg.HistoryRetentionBlocks
	if retention == 0 || s.blockNum <= retention {
		return
	}
	cutoff := s.blockNum - retention

	i := 0
	for ; i < len(s.minedOrder); i++ {
		if s.minedOrder[i].block >= cutoff {
			break
		}
		h := s.minedOrder[i].hash
		delete(s.receipts, h)
		delete(s.minedTxs, h)
	}
	if i > 0 {
		s.minedOrder = append([]minedRef(nil), s.minedOrder[i:]...)
	}

	for addr, hist := range s.nonceHistory {
		s.nonceHistory[addr] = pruneNonceHistory(hist, cutoff)
	}
}

func (s *SimChain) rebuildMinedOrderLocked() {
	s.minedOrder = make([]minedRef, 0, len(s.receipts))
	for hash, rec := range s.receipts {
		s.minedOrder = append(s.minedOrder, minedRef{block: rec.includedAt, hash: hash})
	}
	sort.Slice(s.minedOrder, func(i, j int) bool {
		if s.minedOrder[i].block == s.minedOrder[j].block {
			return s.minedOrder[i].hash.Hex() < s.minedOrder[j].hash.Hex()
		}
		return s.minedOrder[i].block < s.minedOrder[j].block
	})
}

func (s *SimChain) balanceOf(sender common.Address) *big.Int {
	bal, ok := s.balances[sender]
	if !ok {
		return new(big.Int)
	}
	return new(big.Int).Set(bal)
}

func (s *SimChain) injectErr(method string) error {
	s.errMu.Lock()
	defer s.errMu.Unlock()

	for len(s.errInjections[method]) > 0 {
		inj := s.errInjections[method][0]
		if inj.count <= 0 {
			s.errInjections[method] = s.errInjections[method][1:]
			continue
		}
		inj.count--
		s.errInjections[method][0] = inj
		if inj.count == 0 {
			s.errInjections[method] = s.errInjections[method][1:]
		}
		return inj.err
	}
	return nil
}

func (s *SimChain) latestBlock() *simBlock {
	if len(s.blocks) == 0 {
		return nil
	}
	return s.blocks[len(s.blocks)-1]
}

func (s *SimChain) blockByNumber(number uint64) (*simBlock, bool) {
	for _, block := range s.blocks {
		if block.number == number {
			return block, true
		}
	}
	return nil, false
}

func (s *SimChain) headerFromBlock(block *simBlock) *types.Header {
	return &types.Header{
		Number:     new(big.Int).SetUint64(block.number),
		Time:       block.time,
		BaseFee:    new(big.Int).Set(block.baseFee),
		GasLimit:   block.gasLimit,
		GasUsed:    block.gasUsed,
		Difficulty: big.NewInt(0),
	}
}

func (s *SimChain) Run(ctx context.Context) {
	for {
		s.mu.Lock()
		delay := s.nextBlockPeriod()
		s.mu.Unlock()

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		s.mu.Lock()
		s.scheduledBlockDelay = delay
		s.mu.Unlock()
		s.CommitBlock()
	}
}

func (s *SimChain) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	s.cancel()
}

func (s *SimChain) checkClosed() error {
	if s.closed {
		return ErrClosed
	}
	return nil
}

func (s *SimChain) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if err := s.checkClosed(); err != nil {
		return err
	}
	if err := s.injectErr("SendTransaction"); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.recordTxReceived()

	sender, err := types.Sender(s.signer, tx)
	if err != nil {
		wrapped := fmt.Errorf("invalid transaction signature: %w", err)
		s.recordTxRejected(wrapped)
		s.logger.Info("transaction rejected", append(txLogFields(tx, common.Address{}), "reason", wrapped.Error())...)
		return wrapped
	}

	entry := &poolEntry{
		tx:      tx,
		sender:  sender,
		addedAt: s.blockNum,
	}

	replaced := s.pool.getBySenderNonce(sender, tx.Nonce()) != nil
	if err := s.pool.add(entry, s.baseFee, s.minMempoolTip, s.confirmedNonce(sender), s.balanceOf(sender)); err != nil {
		s.recordTxRejected(err)
		s.logger.Info("transaction rejected", append(txLogFields(tx, sender), "reason", err.Error())...)
		return err
	}

	s.recordTxAccepted(replaced)
	fields := append(txLogFields(tx, sender), "replaced", replaced, "mempool_size", s.pool.size())
	s.logger.Info("transaction accepted", fields...)
	return nil
}

func (s *SimChain) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	if err := s.injectErr("TransactionReceipt"); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.receipts[txHash]
	if !ok {
		return nil, ethereum.NotFound
	}

	if s.cfg.ReceiptAvailDelay > 0 {
		blocksSinceInclusion := s.blockNum - rec.includedAt
		if blocksSinceInclusion < s.cfg.ReceiptAvailDelay {
			return nil, ethereum.NotFound
		}
	}

	return rec.receipt, nil
}

func (s *SimChain) TransactionByHash(ctx context.Context, hash common.Hash) (*types.Transaction, bool, error) {
	if err := s.checkClosed(); err != nil {
		return nil, false, err
	}
	if err := s.injectErr("TransactionByHash"); err != nil {
		return nil, false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if entry := s.pool.getByHash(hash); entry != nil {
		return entry.tx, true, nil
	}
	if tx, ok := s.minedTxs[hash]; ok {
		return tx, false, nil
	}
	return nil, false, ethereum.NotFound
}

func (s *SimChain) BlockNumber(ctx context.Context) (uint64, error) {
	if err := s.checkClosed(); err != nil {
		return 0, err
	}
	if err := s.injectErr("BlockNumber"); err != nil {
		return 0, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.blockNum, nil
}

func (s *SimChain) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	if err := s.injectErr("HeaderByNumber"); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var blockNum uint64
	if number == nil {
		blockNum = s.blockNum
	} else {
		blockNum = number.Uint64()
	}

	block, ok := s.blockByNumber(blockNum)
	if !ok {
		return nil, ethereum.NotFound
	}
	return s.headerFromBlock(block), nil
}

func (s *SimChain) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	if err := s.checkClosed(); err != nil {
		return 0, err
	}
	if err := s.injectErr("PendingNonceAt"); err != nil {
		return 0, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.pool.pendingNonce(account, s.confirmedNonce(account)), nil
}

func (s *SimChain) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	if err := s.checkClosed(); err != nil {
		return 0, err
	}
	if err := s.injectErr("NonceAt"); err != nil {
		return 0, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if blockNumber == nil {
		return s.confirmedNonce(account), nil
	}
	return s.nonceAtBlock(account, blockNumber.Uint64()), nil
}

func (s *SimChain) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	if err := s.checkClosed(); err != nil {
		return 0, err
	}
	if err := s.injectErr("EstimateGas"); err != nil {
		return 0, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.estimateGas, nil
}

func (s *SimChain) SuggestedFeeAndTipsFromHistory(ctx context.Context, lastBlock *big.Int) (*transaction.FeeHistorySuggestedFeeAndTips, error) {
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	if err := s.injectErr("SuggestedFeeAndTipsFromHistory"); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	fh, err := s.feeHistoryLocked(lastBlock, s.cfg.FeeHistoryDepth, []float64{10, 50, 90})
	if err != nil {
		return nil, err
	}
	return suggestedFeesFromFeeHistory(fh)
}

func (s *SimChain) SuggestedFeeAndTip(ctx context.Context, gasPrice *big.Int, boostPercent int) (*big.Int, *big.Int, error) {
	if err := s.checkClosed(); err != nil {
		return nil, nil, err
	}
	if err := s.injectErr("SuggestedFeeAndTip"); err != nil {
		return nil, nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if gasPrice != nil {
		block := s.latestBlock()
		if block == nil || block.baseFee == nil {
			return new(big.Int).Set(gasPrice), new(big.Int).Set(gasPrice), nil
		}
		if gasPrice.Cmp(block.baseFee) < 0 {
			return nil, nil, fmt.Errorf("specified gas price %s is below current base fee %s", gasPrice, block.baseFee)
		}
		gasTipCap := new(big.Int).Sub(gasPrice, block.baseFee)
		return new(big.Int).Set(gasPrice), gasTipCap, nil
	}

	fh, err := s.feeHistoryLocked(nil, s.cfg.FeeHistoryDepth, []float64{10, 50, 90})
	if err != nil {
		return nil, nil, err
	}
	tips, err := suggestedFeesFromFeeHistory(fh)
	if err != nil {
		return nil, nil, err
	}

	gasTipCap := new(big.Int).Set(tips.MarketTip)
	if boostPercent > 0 {
		multiplier := big.NewInt(int64(100 + boostPercent))
		gasTipCap.Mul(gasTipCap, multiplier).Div(gasTipCap, big.NewInt(100))
	}
	if gasTipCap.Cmp(s.minMempoolTip) < 0 {
		gasTipCap.Set(s.minMempoolTip)
	}

	gasFeeCap := new(big.Int).Mul(s.baseFee, big.NewInt(2))
	gasFeeCap.Add(gasFeeCap, gasTipCap)

	return gasFeeCap, gasTipCap, nil
}

func (s *SimChain) FeeHistory(ctx context.Context, blockCount uint64, lastBlock *big.Int, rewardPercentiles []float64) (*ethereum.FeeHistory, error) {
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	if err := s.injectErr("FeeHistory"); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.feeHistoryLocked(lastBlock, int(blockCount), rewardPercentiles)
}

func (s *SimChain) ChainID(ctx context.Context) (*big.Int, error) {
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	return new(big.Int).Set(s.cfg.ChainID), nil
}

func (s *SimChain) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	if err := s.injectErr("BalanceAt"); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.balanceOf(account), nil
}

func (s *SimChain) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	if err := s.checkClosed(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	fh, err := s.feeHistoryLocked(nil, s.cfg.FeeHistoryDepth, []float64{10, 50, 90})
	if err != nil {
		return nil, err
	}
	tips, err := suggestedFeesFromFeeHistory(fh)
	if err != nil {
		return nil, err
	}
	return new(big.Int).Set(tips.MarketTip), nil
}

func (s *SimChain) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	return nil, ErrNotImplemented
}

func (s *SimChain) CodeAt(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
	return nil, ErrNotImplemented
}

func (s *SimChain) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	return nil, ErrNotImplemented
}
