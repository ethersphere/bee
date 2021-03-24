// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction"
	"github.com/ethersphere/go-sw3-abi/sw3abi"
)

var (
	erc20ABI = transaction.ParseABIUnchecked(sw3abi.ERC20ABIv0_3_1)
)

type transferEvent struct {
	From  common.Address
	To    common.Address
	Value *big.Int
}

func newTransferLog(address common.Address, from common.Address, to common.Address, value *big.Int) *types.Log {
	return &types.Log{
		Topics: []common.Hash{
			erc20ABI.Events["Transfer"].ID,
			from.Hash(),
			to.Hash(),
		},
		Data:    value.FillBytes(make([]byte, 32)),
		Address: address,
	}
}

func TestParseEvent(t *testing.T) {
	from := common.HexToAddress("00")
	to := common.HexToAddress("01")
	value := big.NewInt(0)

	t.Run("ok", func(t *testing.T) {
		var event transferEvent
		err := transaction.ParseEvent(&erc20ABI, "Transfer", &event, *newTransferLog(common.Address{}, from, to, value))
		if err != nil {
			t.Fatal(err)
		}

		if event.From != from {
			t.Fatalf("parsed wrong from. wanted %x, got %x", from, event.From)
		}

		if event.To != to {
			t.Fatalf("parsed wrong to. wanted %x, got %x", to, event.To)
		}

		if value.Cmp(event.Value) != 0 {
			t.Fatalf("parsed wrong value. wanted %d, got %d", value, event.Value)
		}
	})

	t.Run("no topic", func(t *testing.T) {
		var event transferEvent
		err := transaction.ParseEvent(&erc20ABI, "Transfer", &event, types.Log{
			Topics: []common.Hash{},
		})
		if !errors.Is(err, transaction.ErrNoTopic) {
			t.Fatalf("expected error %v, got %v", transaction.ErrNoTopic, err)
		}
	})
}

func TestFindSingleEvent(t *testing.T) {
	contractAddress := common.HexToAddress("abcd")
	from := common.HexToAddress("00")
	to := common.HexToAddress("01")
	value := big.NewInt(0)

	t.Run("ok", func(t *testing.T) {
		var event transferEvent
		err := transaction.FindSingleEvent(
			&erc20ABI,
			&types.Receipt{
				Logs: []*types.Log{
					newTransferLog(from, to, from, value),                 // event from different contract
					{Topics: []common.Hash{{}}, Address: contractAddress}, // different event from same contract
					newTransferLog(contractAddress, from, to, value),
				},
				Status: 1,
			},
			contractAddress,
			erc20ABI.Events["Transfer"],
			&event,
		)
		if err != nil {
			t.Fatal(err)
		}

		if event.From != from {
			t.Fatalf("parsed wrong from. wanted %x, got %x", from, event.From)
		}

		if event.To != to {
			t.Fatalf("parsed wrong to. wanted %x, got %x", to, event.To)
		}

		if value.Cmp(event.Value) != 0 {
			t.Fatalf("parsed wrong value. wanted %d, got %d", value, event.Value)
		}
	})

	t.Run("not found", func(t *testing.T) {
		var event transferEvent
		err := transaction.FindSingleEvent(
			&erc20ABI,
			&types.Receipt{
				Logs: []*types.Log{
					newTransferLog(from, to, from, value),                 // event from different contract
					{Topics: []common.Hash{{}}, Address: contractAddress}, // different event from same contract
				},
				Status: 1,
			},
			contractAddress,
			erc20ABI.Events["Transfer"],
			&event,
		)
		if !errors.Is(err, transaction.ErrEventNotFound) {
			t.Fatalf("wanted error %v, got %v", transaction.ErrEventNotFound, err)
		}
	})

	t.Run("Reverted", func(t *testing.T) {
		var event transferEvent
		err := transaction.FindSingleEvent(
			&erc20ABI,
			&types.Receipt{Status: 0},
			contractAddress,
			erc20ABI.Events["Transfer"],
			&event,
		)
		if !errors.Is(err, transaction.ErrTransactionReverted) {
			t.Fatalf("wanted error %v, got %v", transaction.ErrTransactionReverted, err)
		}
	})
}
