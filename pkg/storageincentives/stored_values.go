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

func saveSample(store storage.StateStorer, sample sampleData, round uint64) error {
	return store.Put(sampleStorageKey(round), sample)
}

func getSample(store storage.StateStorer, round uint64) (sampleData, error) {
	sample := sampleData{}
	err := store.Get(sampleStorageKey(round), &sample)

	return sample, err
}

func saveCommitKey(store storage.StateStorer, commitKey []byte, round uint64) error {
	return store.Put(commitKeyStorageKey(round), commitKey)
}

func getCommitKey(store storage.StateStorer, round uint64) ([]byte, error) {
	commitKey := make([]byte, swarm.HashSize)
	err := store.Get(commitKeyStorageKey(round), &commitKey)

	return commitKey, err
}

func saveRevealRound(store storage.StateStorer, round uint64) error {
	return store.Put(revealRoundStorageKey(round), true)
}

func getRevealRound(store storage.StateStorer, round uint64) error {
	value := false
	err := store.Get(revealRoundStorageKey(round), &value)

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
