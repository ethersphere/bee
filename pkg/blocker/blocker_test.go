// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package blocker_test

import (
	"io/ioutil"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"

	"github.com/ethersphere/bee/pkg/blocker"
)

func TestBlocksAfterFlagTimeout(t *testing.T) {

	blocked := make(map[string]time.Duration)

	mock := mockBlockLister(func(a swarm.Address, d time.Duration, r string) error {
		blocked[a.ByteString()] = d
		return nil
	})

	logger := logging.New(ioutil.Discard, 0)

	flagTime := time.Millisecond * 50
	checkTime := time.Millisecond * 75
	blockTime := time.Millisecond * 100

	b := blocker.New(mock, flagTime, blockTime, logger)

	addr := test.RandomAddress()
	b.Flag(addr)

	if _, ok := blocked[addr.ByteString()]; ok {
		t.Fatal("blocker did not wait flag duration")
	}

	midway := time.After(flagTime / 2)
	check := time.After(checkTime)

	<-midway
	b.Flag(addr) // check thats this flag call does not overide previous call
	<-check

	blockedTime, ok := blocked[addr.ByteString()]
	if !ok {
		t.Fatal("address should be blocked")
	}

	if blockedTime != blockTime {
		t.Fatalf("block time: want %v, got %v", blockTime, blockedTime)
	}
}

func TestUnflagBeforeBlock(t *testing.T) {

	blocked := make(map[string]time.Duration)

	mock := mockBlockLister(func(a swarm.Address, d time.Duration, r string) error {
		blocked[a.ByteString()] = d
		return nil
	})

	logger := logging.New(ioutil.Discard, 0)

	flagTime := time.Millisecond * 50
	checkTime := time.Millisecond * 75
	blockTime := time.Millisecond * 100

	b := blocker.New(mock, flagTime, blockTime, logger)

	addr := test.RandomAddress()
	b.Flag(addr)

	if _, ok := blocked[addr.ByteString()]; ok {
		t.Fatal("blocker did not wait flag duration")
	}

	b.Unflag(addr)

	time.Sleep(checkTime)

	_, ok := blocked[addr.ByteString()]
	if ok {
		t.Fatal("address should not be blocked")
	}
}

type blocklister struct {
	blocklistFunc func(swarm.Address, time.Duration, string) error
}

func mockBlockLister(f func(swarm.Address, time.Duration, string) error) *blocklister {
	return &blocklister{
		blocklistFunc: f,
	}
}

func (b *blocklister) Blocklist(addr swarm.Address, t time.Duration, r string) error {
	return b.blocklistFunc(addr, t, r)
}
