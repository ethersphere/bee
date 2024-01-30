// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/storageutil"
)

type (
	// StepFn is a function that migrates the storage to the next version
	StepFn func() error
	// Steps is a map of versions and their migration functions
	Steps = map[uint64]StepFn
)

var (
	// errStorageVersionItemUnmarshalInvalidSize is returned when trying
	// to unmarshal buffer that is not of size storageVersionItemSize.
	errStorageVersionItemUnmarshalInvalidSize = errors.New("unmarshal StorageVersionItem: invalid size")
)

// Migrate migrates the storage to the latest version.
// The steps are separated by groups so different lists of steps can run individually, for example,
// two groups of migrations that run before and after the storer is initialized.
func Migrate(s storage.IndexStore, group string, sm Steps) error {
	if err := ValidateVersions(sm); err != nil {
		return err
	}

	currentVersion, err := Version(s, group)
	if err != nil {
		return err
	}

	for nextVersion := currentVersion + 1; ; nextVersion++ {
		stepFn, ok := sm[nextVersion]
		if !ok {
			return nil
		}
		err := stepFn()
		if err != nil {
			return err
		}
		err = setVersion(s, nextVersion, group)
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
	for version := range sm {
		versions[i] = int(version)
		i++
	}
	sort.Ints(versions)

	if (versions[i-1] - versions[0]) == i-1 {
		return nil
	}
	return fmt.Errorf("missing versions")
}

var _ storage.Item = (*StorageVersionItem)(nil)

// storageVersionItemSize is the size of the marshaled storage version item.
const storageVersionItemSize = 8

type StorageVersionItem struct {
	Version uint64
	Group   string
}

// ID implements the storage.Item interface.
func (s *StorageVersionItem) ID() string {
	return "storage_version"
}

// Namespace implements the storage.Item interface.
func (s StorageVersionItem) Namespace() string {
	return s.Group
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

// Clone implements the storage.Item interface.
func (s *StorageVersionItem) Clone() storage.Item {
	if s == nil {
		return nil
	}
	return &StorageVersionItem{
		Version: s.Version,
	}
}

// Clone implements the storage.Item interface.
func (s StorageVersionItem) String() string {
	return storageutil.JoinFields(s.Namespace(), s.ID())
}

// Version returns the current version of the storage
func Version(s storage.Reader, group string) (uint64, error) {
	item := StorageVersionItem{Group: group}
	err := s.Get(&item)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return item.Version, nil
}

// setVersion sets the current version of the storage
func setVersion(s storage.Writer, v uint64, g string) error {
	return s.Put(&StorageVersionItem{Version: v, Group: g})
}

// LatestVersion returns latest version from supplied migration steps.
func LatestVersion(sm Steps) uint64 {
	var latest uint64

	for version := range sm {
		if version > latest {
			latest = version
		}
	}

	return latest
}
