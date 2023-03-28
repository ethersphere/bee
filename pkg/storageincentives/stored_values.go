// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"strconv"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	sampleStorageKeyPrefix      = "storage_incentives_sample_"
	commitKeyStorageKeyPrefix   = "storage_incentives_commit_key_"
	revealRoundStorageKeyPrefix = "storage_incentives_reveal_round_"
)

type Sample struct {
	ReserveSample storage.Sample
	StorageRadius uint8
}

func saveSample(store storage.StateStorer, sample Sample, round uint64) error {
	key := sampleStorageKey(round)
	return store.Put(key, sample)
}

func getSample(store storage.StateStorer, round uint64) (Sample, error) {
	key := sampleStorageKey(round)
	sample := &Sample{}
	err := store.Get(key, sample)

	return *sample, err
}

func saveCommitKey(store storage.StateStorer, commitKey []byte, round uint64) error {
	key := commitKeyStorageKey(round)
	return store.Put(key, commitKey)
}

func getCommitKey(store storage.StateStorer, round uint64) ([]byte, error) {
	key := commitKeyStorageKey(round)
	commitKey := make([]byte, swarm.HashSize)
	err := store.Get(key, &commitKey)

	return commitKey, err
}

func saveRevealRound(store storage.StateStorer, round uint64) error {
	key := revealRoundStorageKey(round)
	return store.Put(key, true)
}

func getRevealRound(store storage.StateStorer, round uint64) error {
	key := revealRoundStorageKey(round)
	value := false
	err := store.Get(key, &value)

	return err
}

func sampleStorageKey(round uint64) string {
	return sampleStorageKeyPrefix + strconv.FormatUint(round, 10)
}

func commitKeyStorageKey(round uint64) string {
	return commitKeyStorageKeyPrefix + strconv.FormatUint(round, 10)
}

func revealRoundStorageKey(round uint64) string {
	return revealRoundStorageKeyPrefix + strconv.FormatUint(round, 10)
}
