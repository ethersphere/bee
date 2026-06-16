// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethersphere/bee/v2/pkg/chainsim"
)

const clientVersion = "chainsim/0.1.0"

type rpcServices struct {
	sim     *chainsim.SimChain
	chainID int64
}

func newRPCServer(sim *chainsim.SimChain, chainID int64) *http.Server {
	services := &rpcServices{sim: sim, chainID: chainID}

	server := rpc.NewServer()
	server.RegisterName("eth", &ethAPI{services})
	server.RegisterName("web3", &web3API{})
	server.RegisterName("net", &netAPI{chainID: chainID})

	return &http.Server{
		Handler: server,
	}
}

type ethAPI struct {
	s *rpcServices
}

type web3API struct{}

type netAPI struct {
	chainID int64
}

func (api *web3API) ClientVersion() string {
	return clientVersion
}

func (api *netAPI) Version() string {
	return fmt.Sprintf("%d", api.chainID)
}

func (api *ethAPI) ChainId() hexutil.Uint64 {
	return hexutil.Uint64(api.s.chainID)
}

func (api *ethAPI) BlockNumber() (hexutil.Uint64, error) {
	num, err := api.s.sim.BlockNumber(context.Background())
	if err != nil {
		return 0, err
	}
	return hexutil.Uint64(num), nil
}

func (api *ethAPI) GetBlockByNumber(_ context.Context, number rpc.BlockNumber, _ bool) (map[string]interface{}, error) {
	blockNum, err := resolveBlockNumber(number)
	if err != nil {
		return nil, err
	}

	header, err := api.s.sim.HeaderByNumber(context.Background(), blockNum)
	if err != nil {
		return nil, err
	}

	return headerToRPCMap(header), nil
}

func (api *ethAPI) GetTransactionCount(_ context.Context, address common.Address, blockNrOrHash rpc.BlockNumberOrHash) (*hexutil.Uint64, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok && blockNr == rpc.PendingBlockNumber {
		nonce, err := api.s.sim.PendingNonceAt(context.Background(), address)
		if err != nil {
			return nil, err
		}
		v := hexutil.Uint64(nonce)
		return &v, nil
	}

	blockNum, err := blockNrOrHashToNumber(blockNrOrHash)
	if err != nil {
		return nil, err
	}

	nonce, err := api.s.sim.NonceAt(context.Background(), address, blockNum)
	if err != nil {
		return nil, err
	}
	v := hexutil.Uint64(nonce)
	return &v, nil
}

func (api *ethAPI) GetBalance(_ context.Context, address common.Address, blockNrOrHash rpc.BlockNumberOrHash) (*hexutil.Big, error) {
	blockNum, err := blockNrOrHashToNumber(blockNrOrHash)
	if err != nil {
		return nil, err
	}

	balance, err := api.s.sim.BalanceAt(context.Background(), address, blockNum)
	if err != nil {
		return nil, err
	}
	v := hexutil.Big(*balance)
	return &v, nil
}

func (api *ethAPI) EstimateGas(_ context.Context, _ interface{}, _ ...rpc.BlockNumber) (hexutil.Uint64, error) {
	gas, err := api.s.sim.EstimateGas(context.Background(), ethereum.CallMsg{})
	if err != nil {
		return 0, err
	}
	return hexutil.Uint64(gas), nil
}

func (api *ethAPI) SendRawTransaction(ctx context.Context, input hexutil.Bytes) (common.Hash, error) {
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(input); err != nil {
		return common.Hash{}, err
	}
	if err := api.s.sim.SendTransaction(ctx, tx); err != nil {
		return common.Hash{}, err
	}
	return tx.Hash(), nil
}

func (api *ethAPI) GetTransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	receipt, err := api.s.sim.TransactionReceipt(ctx, hash)
	if err != nil {
		return nil, err
	}
	return receipt, nil
}

func (api *ethAPI) GetTransactionByHash(ctx context.Context, hash common.Hash) (interface{}, error) {
	tx, pending, err := api.s.sim.TransactionByHash(ctx, hash)
	if err != nil {
		return nil, err
	}

	from, err := types.Sender(api.s.sim.Signer(), tx)
	if err != nil {
		return nil, fmt.Errorf("recover sender: %w", err)
	}

	if pending {
		return marshalPendingTransaction(tx, from)
	}

	receipt, err := api.s.sim.TransactionReceipt(ctx, hash)
	if err != nil {
		return nil, err
	}

	return marshalMinedTransaction(tx, from, receipt)
}

func (api *ethAPI) FeeHistory(_ context.Context, blockCount math.HexOrDecimal64, lastBlock rpc.BlockNumber, rewardPercentiles []float64) (*feeHistoryResult, error) {
	var last *big.Int
	if lastBlock != rpc.LatestBlockNumber && lastBlock != rpc.PendingBlockNumber {
		last = big.NewInt(lastBlock.Int64())
	}

	fh, err := api.s.sim.FeeHistory(context.Background(), uint64(blockCount), last, rewardPercentiles)
	if err != nil {
		return nil, err
	}

	result := &feeHistoryResult{
		OldestBlock:  (*hexutil.Big)(fh.OldestBlock),
		GasUsedRatio: fh.GasUsedRatio,
	}

	if fh.Reward != nil {
		result.Reward = make([][]*hexutil.Big, len(fh.Reward))
		for i, rewards := range fh.Reward {
			result.Reward[i] = make([]*hexutil.Big, len(rewards))
			for j, reward := range rewards {
				result.Reward[i][j] = (*hexutil.Big)(reward)
			}
		}
	}

	if fh.BaseFee != nil {
		result.BaseFee = make([]*hexutil.Big, len(fh.BaseFee))
		for i, fee := range fh.BaseFee {
			result.BaseFee[i] = (*hexutil.Big)(fee)
		}
	}

	return result, nil
}

func (api *ethAPI) MaxPriorityFeePerGas(_ context.Context) (*hexutil.Big, error) {
	tip, err := api.s.sim.SuggestGasTipCap(context.Background())
	if err != nil {
		return nil, err
	}
	v := hexutil.Big(*tip)
	return &v, nil
}

func (api *ethAPI) GasPrice(_ context.Context) (*hexutil.Big, error) {
	feeCap, _, err := api.s.sim.SuggestedFeeAndTip(context.Background(), nil, 0)
	if err != nil {
		return nil, err
	}
	v := hexutil.Big(*feeCap)
	return &v, nil
}

type feeHistoryResult struct {
	OldestBlock  *hexutil.Big     `json:"oldestBlock"`
	Reward       [][]*hexutil.Big `json:"reward,omitempty"`
	BaseFee      []*hexutil.Big   `json:"baseFeePerGas,omitempty"`
	GasUsedRatio []float64        `json:"gasUsedRatio"`
}

func resolveBlockNumber(number rpc.BlockNumber) (*big.Int, error) {
	switch number {
	case rpc.LatestBlockNumber, rpc.PendingBlockNumber, rpc.FinalizedBlockNumber, rpc.SafeBlockNumber:
		return nil, nil
	case rpc.EarliestBlockNumber:
		return big.NewInt(0), nil
	default:
		if number.Int64() < 0 {
			return nil, ethereum.NotFound
		}
		return big.NewInt(number.Int64()), nil
	}
}

func blockNrOrHashToNumber(blockNrOrHash rpc.BlockNumberOrHash) (*big.Int, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return resolveBlockNumber(blockNr)
	}
	return nil, nil
}

func headerToRPCMap(header *types.Header) map[string]interface{} {
	return map[string]interface{}{
		"number":           (*hexutil.Big)(header.Number),
		"hash":             header.Hash(),
		"parentHash":       header.ParentHash,
		"nonce":            header.Nonce,
		"sha3Uncles":       header.UncleHash,
		"logsBloom":        header.Bloom,
		"transactionsRoot": header.TxHash,
		"stateRoot":        header.Root,
		"receiptsRoot":     header.ReceiptHash,
		"miner":            header.Coinbase,
		"difficulty":       (*hexutil.Big)(header.Difficulty),
		"totalDifficulty":  (*hexutil.Big)(header.Difficulty),
		"extraData":        hexutil.Bytes(header.Extra),
		"size":             hexutil.Uint64(0),
		"gasLimit":         hexutil.Uint64(header.GasLimit),
		"gasUsed":          hexutil.Uint64(header.GasUsed),
		"timestamp":        hexutil.Uint64(header.Time),
		"baseFeePerGas":    (*hexutil.Big)(header.BaseFee),
		"transactions":     []interface{}{},
		"uncles":           []interface{}{},
	}
}

func marshalPendingTransaction(tx *types.Transaction, from common.Address) (map[string]interface{}, error) {
	result, err := marshalTransactionFields(tx)
	if err != nil {
		return nil, err
	}
	result["from"] = from
	result["hash"] = tx.Hash()
	return result, nil
}

func marshalMinedTransaction(tx *types.Transaction, from common.Address, receipt *types.Receipt) (map[string]interface{}, error) {
	result, err := marshalTransactionFields(tx)
	if err != nil {
		return nil, err
	}
	result["from"] = from
	result["hash"] = tx.Hash()
	result["blockHash"] = receipt.BlockHash
	result["blockNumber"] = (*hexutil.Big)(receipt.BlockNumber)
	result["transactionIndex"] = hexutil.Uint64(0)
	return result, nil
}

func marshalTransactionFields(tx *types.Transaction) (map[string]interface{}, error) {
	raw, err := json.Marshal(tx)
	if err != nil {
		return nil, fmt.Errorf("marshal transaction: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("unmarshal transaction map: %w", err)
	}
	return result, nil
}
