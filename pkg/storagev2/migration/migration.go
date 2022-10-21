// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"encoding/binary"
	"errors"
	"fmt"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"sort"
)

type (
	// StepFn is a function that migrates the storage to the next version
	StepFn func(storage.Store) error
	// Steps is a map of versions and their migration functions
	Steps = map[uint64]StepFn
)

var (
	errStorageVersionItemUnmarshalInvalidSize = errors.New("unmarshal StorageVersionItem: invalid size")
)

// Migrate migrates the storage to the latest version
func Migrate(s storage.Store, sm Steps) error {
	if err := ValidateVersions(sm); err != nil {
		return err
	}

	currentVersion, err := Version(s)
	if err != nil {
		return err
	}

	for nextVersion := currentVersion + 1; ; nextVersion++ {
		stepFn, ok := sm[nextVersion]
		if !ok {
			return nil
		}
		err := stepFn(s)
		if err != nil {
			return err
		}
		err = SetVersion(s, nextVersion)
		if err != nil {
			return err
		}
	}
}

// ValidateVersions checks versions if they are in order n (where n min version value), n+1, n+2, n+3... (all values are increasing orders)
func ValidateVersions(sm Steps) error {
	if len(sm) == 0 {
		return fmt.Errorf("steps map is empty")
	}
	versions := make([]int, len(sm))
	i := 0
	for key := range sm {
		versions[i] = int(key)
		i++
	}
	sort.Ints(versions)

	if (versions[i-1] - versions[0]) == i-1 {
		return nil
	}
	return fmt.Errorf("missing versions")
}

type StorageVersionItem struct {
	Version uint64
}

const storageVersionItemSize = 8

// ID implements the storage.Item interface.
func (s *StorageVersionItem) ID() string {
	return "storage_version"
}

// Namespace implements the storage.Item interface.
func (s StorageVersionItem) Namespace() string {
	return "migration"
}

// Marshal implements the storage.Item interface.
func (s *StorageVersionItem) Marshal() ([]byte, error) {
	buf := make([]byte, storageVersionItemSize)
	binary.LittleEndian.PutUint64(buf, s.Version)
	return buf, nil
}

// Unmarshal implements the storage.Item interface.
func (s *StorageVersionItem) Unmarshal(bytes []byte) error {
	if len(bytes) != storageVersionItemSize {
		return errStorageVersionItemUnmarshalInvalidSize
	}
	s.Version = binary.LittleEndian.Uint64(bytes)
	return nil
}

func Version(s storage.Store) (uint64, error) {
	item := StorageVersionItem{}
	err := s.Get(&item)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return item.Version, nil
}

func SetVersion(s storage.Store, v uint64) error {
	return s.Put(&StorageVersionItem{Version: v})
}
