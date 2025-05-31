// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// InitStamperStore will create new stamper store with the given path to the
// data directory. When given an empty directory path, the function will instead
// initialize an in-memory state store that will not be persisted.
func InitStamperStore(logger log.Logger, dataDir string, stateStore storage.StateStorer) (storage.Store, error) {
	if dataDir == "" {
		logger.Warning("using in-mem stamper store, no node state will be persisted")
	} else {
		dataDir = filepath.Join(dataDir, "stamperstore")
	}
	stamperStore, err := leveldbstore.New(dataDir, nil)
	if err != nil {
		return nil, err
	}

	return stamperStore, nil
}

const (
	overlayNonce     = "overlayV2_nonce"
	noncedOverlayKey = "nonce-overlay"
)

// checkOverlay checks the overlay is the same as stored in the statestore
func checkOverlay(storer storage.StateStorer, overlay swarm.Address) error {

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

func overlayNonceExists(s storage.StateStorer) ([]byte, bool, error) {
	nonce := make([]byte, 32)
	if err := s.Get(overlayNonce, &nonce); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nonce, false, nil
		}
		return nil, false, err
	}
	return nonce, true, nil
}

func setOverlay(s storage.StateStorer, overlay swarm.Address, nonce []byte) error {
	return errors.Join(
		s.Put(overlayNonce, nonce),
		s.Put(noncedOverlayKey, overlay),
	)
}
