// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"errors"
	"fmt"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"path/filepath"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/metrics"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/statestore/storeadapter"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/cache"
	"github.com/ethersphere/bee/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/pkg/swarm"
)

// InitStateStore will initialize the stateStore with the given path to the
// data directory. When given an empty directory path, the function will instead
// initialize an in-memory state store that will not be persisted.
func InitStateStore(logger log.Logger, dataDir string, cacheCapacity uint64) (storage.StateStorer, metrics.Collector, error) {
	if dataDir == "" {
		logger.Warning("using in-mem state store, no node state will be persisted")
	} else {
		dataDir = filepath.Join(dataDir, "statestore")
	}
	ldb, err := leveldbstore.New(dataDir, &opt.Options{Compression: opt.NoCompression})
	if err != nil {
		return nil, nil, err
	}

	caching := cache.Wrap(ldb, int(cacheCapacity))
	stateStore, err := storeadapter.NewStateStorerAdapter(caching)

	return stateStore, caching, err
}

// InitStamperStore will create new stamper store with the given path to the
// data directory. When given an empty directory path, the function will instead
// initialize an in-memory state store that will not be persisted.
func InitStamperStore(logger log.Logger, dataDir string, stateStore storage.StateStorer) (storage.Store, error) {
	if dataDir == "" {
		logger.Warning("using in-mem stamper store, no node state will be persisted")
	} else {
		dataDir = filepath.Join(dataDir, "stamperstore")
	}
	stamperStore, err := leveldbstore.New(dataDir, &opt.Options{Compression: opt.NoCompression})
	if err != nil {
		return nil, err
	}
	err = migrateStamperData(stateStore, stamperStore)
	if err != nil {
		stamperStore.Close()
		return nil, fmt.Errorf("migrating stamper data: %w", err)
	}
	return stamperStore, nil
}

const noncedOverlayKey = "nonce-overlay"

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
			return overlayNonce, false, nil
		}
		return nil, false, err
	}
	return overlayNonce, true, nil
}

func setOverlayNonce(s storage.StateStorer, overlayNonce []byte) error {
	return s.Put(OverlayNonce, overlayNonce)
}

func migrateStamperData(stateStore storage.StateStorer, stamperStore storage.Store) error {
	var keys []string
	err := stateStore.Iterate("postage", func(key, value []byte) (bool, error) {
		keys = append(keys, string(key))
		st := &postage.StampIssuer{}
		if err := st.UnmarshalBinary(value); err != nil {
			return false, err
		}
		if err := stamperStore.Put(&postage.StampIssuerItem{
			Issuer: st,
		}); err != nil {
			return false, err
		}
		return false, nil
	})
	if err != nil {
		return err
	}

	for _, key := range keys {
		if err = stateStore.Delete(key); err != nil {
			return err
		}
	}
	return nil
}
