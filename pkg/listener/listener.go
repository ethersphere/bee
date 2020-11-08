// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package listener

import (
	"context"
	"math/big"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Listener provides a blockchain event iterator
type Listener interface {
	// - it starts at block from
	// - it terminates  with no error when quit channel is closed
	// - if the update function returns an error, the call returns with that error
	Listen(from uint64, quit chan struct{}, update func(types.Log) error) error
}

var _ Listener = (*EthListener)(nil)

// EthListener can listen to events of contracts on the Ethereum blockchain
type EthListener struct {
	client *ethclient.Client
	addrs  []common.Address
}

// New constructs an EthListener
// - it uses the client to listen to  events emitted by contracts at addresses in addrs
func New(client *ethclient.Client, addrs []common.Address) *EthListener {
	return &EthListener{client, addrs}
}

// Listen is an iterator that calls a function on each event logged by contracts at addrs
// - it starts at block from
// - it terminates  with no error when quit channel is closed
// - if the update function returns an error, the call returns with that error
func (lis *EthListener) Listen(from uint64, quit chan struct{}, f func(types.Log) error) error {
	// need to cancel context even if terminate without error with quit channel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// using an event channel, we subscribe to filter logs emmitted by the postage contract(s)
	// since the last block recorded that had a relevant event
	events := make(chan types.Log)
	sub, err := lis.client.SubscribeFilterLogs(ctx, query(from, lis.addrs...), events)
	if err != nil {
		return err
	}
LOOP:
	for {
		select {
		case err = <-sub.Err(): // subscription error
			break LOOP
		case <-quit: // normal quit
			break LOOP
		case ev := <-events:
			// call function on event
			// if this call returns an error the listen loop terminates
			err = f(ev)
			if err != nil {
				break LOOP
			}
		}
	}
	return err
}

func query(from uint64, addrs ...common.Address) ethereum.FilterQuery {
	return ethereum.FilterQuery{
		Addresses: addrs,
		FromBlock: big.NewInt(0).SetUint64(from),
	}
}
