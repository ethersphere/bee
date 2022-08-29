// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/statestore/leveldb"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// InitStateStore will initialize the stateStore with the given path to the
// data directory. When given an empty directory path, the function will instead
// initialize an in-memory state store that will not be persisted.
func InitStateStore(logger log.Logger, dataDir string) (storage.StateStorer, error) {
	if dataDir == "" {
		logger.Warning("using in-mem state store, no node state will be persisted")
		return leveldb.NewInMemoryStateStore(logger)
	}
	return leveldb.NewStateStore(filepath.Join(dataDir, "statestore"), logger)
}

const secureOverlayKey = "non-mineable-overlay"
const noncedOverlayKey = "nonce-overlay"

func GetExistingOverlay(storer storage.StateStorer) (swarm.Address, error) {
	var storedOverlay swarm.Address
	err := storer.Get(secureOverlayKey, &storedOverlay)
	if err != nil {
		return swarm.ZeroAddress, err
	}

	return storedOverlay, nil
}

// CheckOverlayWithStore checks the overlay is the same as stored in the statestore
func CheckOverlayWithStore(overlay swarm.Address, storer storage.StateStorer) error {

	var storedOverlay swarm.Address
	err := storer.Get(noncedOverlayKey, &storedOverlay)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return err
		}
		return storer.Put(noncedOverlayKey, overlay)
	}

	if !storedOverlay.Equal(overlay) {
		return fmt.Errorf("overlay address changed. was %s before but now is %s", storedOverlay, overlay)
	}

	return nil
}

// SetOverlayInStore sets the overlay stored in the statestore (for purpose of overlay migration)
func SetOverlayInStore(overlay swarm.Address, storer storage.StateStorer) error {
	return storer.Put(noncedOverlayKey, overlay)
}

const OverlayNonce = "overlayV2_nonce"

func overlayNonceExists(s storage.StateStorer) ([]byte, bool, error) {
	overlayNonce := make([]byte, 32)
	if err := s.Get(OverlayNonce, &overlayNonce); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return overlayNonce, true, nil
}

func setOverlayNonce(s storage.StateStorer, overlayNonce []byte) error {
	return s.Put(OverlayNonce, overlayNonce)
}
