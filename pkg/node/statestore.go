// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/statestore/leveldb"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"

	ldb "github.com/syndtr/goleveldb/leveldb"
	ldbs "github.com/syndtr/goleveldb/leveldb/storage"
)

type MockStore struct {
	ldb *ldb.DB
}

func (m *MockStore) Get(key string, i interface{}) (err error) {
	return nil
}
func (m *MockStore) Put(key string, i interface{}) (err error) {
	return nil
}
func (m *MockStore) Delete(key string) (err error) {
	return nil
}
func (m *MockStore) Iterate(prefix string, iterFunc storage.StateIterFunc) (err error) {
	return nil
}

// DB returns the underlying DB storage.
func (m *MockStore) DB() *ldb.DB {
	return m.ldb
}
func (m *MockStore) Close() error {
	return nil
}

// InitStateStore will initialize the stateStore with the given path to the
// data directory. When given an empty directory path, the function will instead
// initialize an in-memory state store that will not be persisted.
func InitStateStore(log logging.Logger, dataDir string) (ret storage.StateStorer, err error) {
	if dataDir == "" {
		ldb, err := ldb.Open(ldbs.NewMemStorage(), nil)
		log.Warning("using in-mem state store, no node state will be persisted")
		return &MockStore{ldb: ldb}, err
	}
	return leveldb.NewStateStore(filepath.Join(dataDir, "statestore"), log)
}

const overlayKey = "overlay"
const secureOverlayKey = "non-mineable-overlay"

// CheckOverlayWithStore checks the overlay is the same as stored in the statestore
func CheckOverlayWithStore(overlay swarm.Address, storer storage.StateStorer) error {

	// migrate overlay key to new key
	err := storer.Delete(overlayKey)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return err
	}

	var storedOverlay swarm.Address
	err = storer.Get(secureOverlayKey, &storedOverlay)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return err
		}
		return storer.Put(secureOverlayKey, overlay)
	}

	if !storedOverlay.Equal(overlay) {
		return fmt.Errorf("overlay address changed. was %s before but now is %s", storedOverlay, overlay)
	}

	return nil
}
