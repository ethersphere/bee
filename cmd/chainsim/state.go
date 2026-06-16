// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethersphere/bee/v2/pkg/chainsim"
)

const (
	stateFileName   = "state.json"
	stateTempName   = "state.json.tmp"
	stateBackupName = "state.json.bak"
	stateSaveEvery  = 10
)

type stateStore struct {
	dir string
}

func newStateStore(dir string) (*stateStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	return &stateStore{dir: dir}, nil
}

func (s *stateStore) path(name string) string {
	return filepath.Join(s.dir, name)
}

func (s *stateStore) exists() bool {
	_, err := os.Stat(s.path(stateFileName))
	return err == nil
}

func (s *stateStore) load() (chainsim.Snapshot, error) {
	raw, err := os.ReadFile(s.path(stateFileName))
	if err != nil {
		return chainsim.Snapshot{}, fmt.Errorf("read state: %w", err)
	}

	var snap chainsim.Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return chainsim.Snapshot{}, fmt.Errorf("parse state: %w", err)
	}
	return snap, nil
}

func (s *stateStore) save(sim *chainsim.SimChain) error {
	snap := sim.Snapshot()
	raw, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	tmpPath := s.path(stateTempName)
	if err := os.WriteFile(tmpPath, raw, 0o644); err != nil {
		return fmt.Errorf("write temp state: %w", err)
	}

	statePath := s.path(stateFileName)
	if _, err := os.Stat(statePath); err == nil {
		_ = os.Rename(statePath, s.path(stateBackupName))
	}
	if err := os.Rename(tmpPath, statePath); err != nil {
		return fmt.Errorf("commit state: %w", err)
	}
	return nil
}

func attachStateSaver(sim *chainsim.SimChain, store *stateStore) {
	var lastSaved uint64
	sim.SetBlockCommitHook(func(blockNum uint64) {
		if blockNum-lastSaved < stateSaveEvery {
			return
		}
		if err := store.save(sim); err != nil {
			fmt.Fprintf(os.Stderr, "chainsim: save state at block %d: %v\n", blockNum, err)
			return
		}
		lastSaved = blockNum
	})
}
