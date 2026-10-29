// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package depth

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/transaction"
)

// ErrUnexpectedTopic is returned when a log does not carry the expected event id.
var ErrUnexpectedTopic = errors.New("depth: unexpected event topic")

// ErrUnexpectedDataLength is returned when the log data is not exactly two ABI words.
var ErrUnexpectedDataLength = errors.New("depth: unexpected event data length")

// truthSelectedDataLength is the size of the packed (bytes32 hash, uint8 depth) data.
const truthSelectedDataLength = 64

type truthSelectedEvent struct {
	Hash  [32]byte
	Depth uint8
}

// decodeTruthSelected decodes one TruthSelected log. It returns an error when the first
// topic is not the event id, when there are no topics, or when the data does not unpack.
func decodeTruthSelected(a *abi.ABI, topic common.Hash, l types.Log) (sample, error) {
	if len(l.Topics) == 0 {
		return sample{}, transaction.ErrNoTopic
	}
	if l.Topics[0] != topic {
		return sample{}, ErrUnexpectedTopic
	}

	// ParseEvent skips unpacking for empty data and ignores trailing bytes, so the
	// exact length is enforced here.
	if len(l.Data) != truthSelectedDataLength {
		return sample{}, ErrUnexpectedDataLength
	}

	var ev truthSelectedEvent
	if err := transaction.ParseEvent(a, "TruthSelected", &ev, l); err != nil {
		return sample{}, fmt.Errorf("parse TruthSelected: %w", err)
	}

	return sample{
		block:    l.BlockNumber,
		txIndex:  l.TxIndex,
		logIndex: l.Index,
		depth:    ev.Depth,
	}, nil
}
