// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives_test

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storageincentives"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/util/testutil"
)

func TestStorage_Sample(t *testing.T) {
	t.Parallel()

	s := statestore.NewStateStore()

	_, err := storageincentives.GetSample(s, 1)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Error("expected error")
	}

	err = storageincentives.RemoveSample(s, 1)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	savedSample := storageincentives.SampleData{
		ReserveSample: storage.Sample{
			Hash: swarm.RandAddress(t),
		},
		StorageRadius: 3,
	}
	err = storageincentives.SaveSample(s, savedSample, 1)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	sample, err := storageincentives.GetSample(s, 1)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(savedSample, sample) {
		t.Errorf("sample does not match saved sample")
	}

	err = storageincentives.RemoveSample(s, 1)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	_, err = storageincentives.GetSample(s, 1)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Error("expected error")
	}
}

func TestStorage_CommitKey(t *testing.T) {
	t.Parallel()

	s := statestore.NewStateStore()

	_, err := storageincentives.GetCommitKey(s, 1)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Error("expected error")
	}

	err = storageincentives.RemoveCommitKey(s, 1)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	savedKey := testutil.RandBytes(t, swarm.HashSize)
	err = storageincentives.SaveCommitKey(s, savedKey, 1)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	key, err := storageincentives.GetCommitKey(s, 1)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !bytes.Equal(savedKey, key) {
		t.Errorf("key does not match saved key")
	}

	err = storageincentives.RemoveCommitKey(s, 1)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	_, err = storageincentives.GetCommitKey(s, 1)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Error("expected error")
	}
}

func TestStorage_RevealRound(t *testing.T) {
	t.Parallel()

	s := statestore.NewStateStore()

	err := storageincentives.GetRevealRound(s, 1)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Error("expected error")
	}

	err = storageincentives.RemoveRevealRound(s, 1)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	err = storageincentives.SaveRevealRound(s, 1)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	err = storageincentives.GetRevealRound(s, 1)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	err = storageincentives.RemoveRevealRound(s, 1)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	err = storageincentives.GetRevealRound(s, 1)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Error("expected error")
	}
}

func TestStorage_LastPurgedDataRound(t *testing.T) {
	t.Parallel()

	s := statestore.NewStateStore()

	round, err := storageincentives.GetLastPurgedRound(s)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if round != 0 {
		t.Errorf("have %d, want %d", round, 0)
	}

	savedRound := uint64(1)

	err = storageincentives.SaveLastPurgedRound(s, savedRound)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	round, err = storageincentives.GetLastPurgedRound(s)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if round != savedRound {
		t.Errorf("have %d, want %d", round, savedRound)
	}
}
