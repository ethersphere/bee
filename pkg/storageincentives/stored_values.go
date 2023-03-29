// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"errors"
	"strconv"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	sampleStorageKeyPrefix      = "storage_incentives_sample_"
	commitKeyStorageKeyPrefix   = "storage_incentives_commit_key_"
	revealRoundStorageKeyPrefix = "storage_incentives_reveal_round_"
	lastPurgedRoundKey          = "storage_incentives_last_purged_round"
)

func saveSample(store storage.StateStorer, sample sampleData, round uint64) error {
	return store.Put(sampleStorageKey(round), sample)
}

func getSample(store storage.StateStorer, round uint64) (sampleData, error) {
	sample := sampleData{}
	err := store.Get(sampleStorageKey(round), &sample)

	return sample, err
}

func removeSample(store storage.StateStorer, round uint64) error {
	err := store.Delete(sampleStorageKey(round))
	if errors.Is(err, storage.ErrNotFound) {
		return nil // swallow error when nothing was removed
	}
	return err
}

func saveCommitKey(store storage.StateStorer, commitKey []byte, round uint64) error {
	return store.Put(commitKeyStorageKey(round), commitKey)
}

func getCommitKey(store storage.StateStorer, round uint64) ([]byte, error) {
	commitKey := make([]byte, swarm.HashSize)
	err := store.Get(commitKeyStorageKey(round), &commitKey)

	return commitKey, err
}

func removeCommitKey(store storage.StateStorer, round uint64) error {
	err := store.Delete(commitKeyStorageKey(round))
	if errors.Is(err, storage.ErrNotFound) {
		return nil // swallow error when nothing was removed
	}
	return err
}

func saveRevealRound(store storage.StateStorer, round uint64) error {
	return store.Put(revealRoundStorageKey(round), true)
}

func getRevealRound(store storage.StateStorer, round uint64) error {
	value := false
	err := store.Get(revealRoundStorageKey(round), &value)

	return err
}

func removeRevealRound(store storage.StateStorer, round uint64) error {
	err := store.Delete(revealRoundStorageKey(round))
	if errors.Is(err, storage.ErrNotFound) {
		return nil // swallow error when nothing was removed
	}
	return err
}

func saveLastPurgedRound(store storage.StateStorer, round uint64) error {
	return store.Put(lastPurgedRoundKey, round)
}

func getLastPurgedRound(store storage.StateStorer) (uint64, error) {
	var value uint64
	err := store.Get(lastPurgedRoundKey, &value)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return 0, nil
		}

		return 0, err
	}

	return value, nil
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
