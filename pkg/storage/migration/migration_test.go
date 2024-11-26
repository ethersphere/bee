// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"encoding/binary"
	"errors"
	"math"
	"math/rand"
	"strconv"
	"testing"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
	"github.com/ethersphere/bee/v2/pkg/storage/migration"
	"github.com/ethersphere/bee/v2/pkg/storage/storagetest"
	"github.com/ethersphere/bee/v2/pkg/storage/storageutil"
)

var errStep = errors.New("step error")

func TestLatestVersion(t *testing.T) {
	t.Parallel()

	const expectedLatestVersion = 8
	steps := migration.Steps{
		8: func() error { return nil },
		7: func() error { return nil },
		6: func() error { return nil },
	}

	latestVersion := migration.LatestVersion(steps)

	if latestVersion != expectedLatestVersion {
		t.Errorf("got %d, expected %d", latestVersion, expectedLatestVersion)
	}
}

func TestGetSetVersion(t *testing.T) {
	t.Parallel()

	t.Run("Version", func(t *testing.T) {
		t.Parallel()

		s := inmemstore.New()

		gotVersion, err := migration.Version(s, "migration")
		if err != nil {
			t.Errorf("Version() unexpected error: %v", err)
		}
		if gotVersion != 0 {
			t.Errorf("expect version to be 0, got %v", gotVersion)
		}
	})

	t.Run("SetVersion", func(t *testing.T) {
		t.Parallel()

		s := inmemstore.New()

		const version = 10

		err := migration.SetVersion(s, version, "migration")
		if err != nil {
			t.Errorf("SetVersion() unexpected error: %v", err)
		}

		gotVersion, err := migration.Version(s, "migration")
		if err != nil {
			t.Errorf("Version() unexpected error: %v", err)
		}
		if gotVersion != version {
			t.Errorf("expect version to be %d, got %d", version, gotVersion)
		}
	})
}

func TestValidateVersions(t *testing.T) {
	t.Parallel()
	objT1 := &obj{id: 111, val: 1}
	objT2 := &obj{id: 222, val: 2}
	objT3 := &obj{id: 333, val: 3}

	s := inmemstore.New()

	tests := []struct {
		name    string
		input   migration.Steps
		wantErr bool
	}{
		{
			name:    "empty",
			input:   migration.Steps{},
			wantErr: true,
		},
		{
			name: "missing version 3",
			input: migration.Steps{
				1: func() error {
					return s.Put(objT1)
				},
				2: func() error {
					return s.Put(objT2)
				},
				4: func() error {
					return s.Put(objT3)
				},
			},
			wantErr: true,
		},
		{
			name: "not missing",
			input: migration.Steps{
				1: func() error {
					return s.Put(objT1)
				},
				2: func() error {
					return s.Put(objT2)
				},
				3: func() error {
					return s.Put(objT3)
				},
			},
			wantErr: false,
		},
		{
			name: "desc order versions",
			input: migration.Steps{
				3: func() error {
					return s.Put(objT1)
				},
				2: func() error {
					return s.Put(objT2)
				},
				1: func() error {
					return s.Put(objT3)
				},
			},
			wantErr: false,
		},
		{
			name: "desc order version missing",
			input: migration.Steps{
				4: func() error {
					return s.Put(objT1)
				},
				2: func() error {
					return s.Put(objT2)
				},
				1: func() error {
					return s.Put(objT3)
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := migration.ValidateVersions(tt.input); (err != nil) != tt.wantErr {
				t.Errorf("ValidateVersions() unexpected error: %v, wantErr : %v", err, tt.wantErr)
			}
		})
	}
}

func TestMigrate(t *testing.T) {
	t.Parallel()
	objT1 := &obj{id: 111, val: 1}
	objT2 := &obj{id: 222, val: 2}
	objT3 := &obj{id: 333, val: 3}

	t.Run("migration: 0 to 3", func(t *testing.T) {
		t.Parallel()

		s := inmemstore.New()

		steps := migration.Steps{
			1: func() error {
				return s.Put(objT1)
			},
			2: func() error {
				return s.Put(objT2)
			},
			3: func() error {
				return s.Put(objT3)
			},
		}

		if err := migration.Migrate(s, "migration", steps); err != nil {
			t.Errorf("Migrate() unexpected error: %v", err)
		}

		newVersion, err := migration.Version(s, "migration")
		if err != nil {
			t.Errorf("Version() unexpected error: %v", err)
		}
		if newVersion != 3 {
			t.Errorf("new version = %v must be 3", newVersion)
		}

		assertObjectExists(t, s, objT1, objT2, objT3)
	})

	t.Run("migration: 5 to 8", func(t *testing.T) {
		t.Parallel()

		s := inmemstore.New()

		steps := migration.Steps{
			8: func() error {
				return s.Put(objT1)
			},
			7: func() error {
				return s.Put(objT2)
			},
			6: func() error {
				return s.Put(objT3)
			},
		}

		err := migration.SetVersion(s, 5, "migration")
		if err != nil {
			t.Errorf("SetVersion() unexpected error: %v", err)
		}

		if err := migration.Migrate(s, "migration", steps); err != nil {
			t.Errorf("Migrate() unexpected error: %v", err)
		}

		newVersion, err := migration.Version(s, "migration")
		if err != nil {
			t.Errorf("Version() unexpected error: %v", err)
		}
		if newVersion != 8 {
			t.Errorf("new version = %v must be 8", newVersion)
		}

		assertObjectExists(t, s, objT1, objT2, objT3)
	})

	t.Run("migration: 5 to 8 with steps error", func(t *testing.T) {
		t.Parallel()

		s := inmemstore.New()

		steps := migration.Steps{
			8: func() error {
				return s.Put(objT1)
			},
			7: func() error {
				return errStep
			},
			6: func() error {
				return s.Put(objT3)
			},
		}

		err := migration.SetVersion(s, 5, "migration")
		if err != nil {
			t.Errorf("SetVersion() unexpected error: %v", err)
		}

		if err := migration.Migrate(s, "migration", steps); err != nil && !errors.Is(err, errStep) {
			t.Errorf("Migrate() unexpected error: %v", err)
		}

		newVersion, err := migration.Version(s, "migration")
		if err != nil {
			t.Errorf("Version() unexpected error: %v", err)
		}
		// since we have error on step 7, we should be on version 6
		if newVersion != 6 {
			t.Errorf("new version = %v must be 6", newVersion)
		}
	})
}

func assertObjectExists(t *testing.T, s storage.BatchStore, keys ...storage.Key) {
	t.Helper()

	for _, key := range keys {
		if isExist, _ := s.Has(key); !isExist {
			t.Errorf("key = %v doesn't exists", key)
		}
	}
}

func TestTagIDAddressItem_MarshalAndUnmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{
		{
			name: "zero values",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item:    &migration.StorageVersionItem{},
				Factory: func() storage.Item { return new(migration.StorageVersionItem) },
			},
		},
		{
			name: "max value",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item:    &migration.StorageVersionItem{Version: math.MaxUint64},
				Factory: func() storage.Item { return new(migration.StorageVersionItem) },
			},
		},
		{
			name: "invalid size",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item: &storagetest.ItemStub{
					MarshalBuf:   []byte{0xFF},
					UnmarshalBuf: []byte{0xFF},
				},
				Factory:      func() storage.Item { return new(migration.StorageVersionItem) },
				UnmarshalErr: migration.ErrStorageVersionItemUnmarshalInvalidSize,
			},
		},
		{
			name: "random value",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item:    &migration.StorageVersionItem{Version: rand.Uint64()},
				Factory: func() storage.Item { return new(migration.StorageVersionItem) },
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			storagetest.TestItemMarshalAndUnmarshal(t, tc.test)
		})
	}
}

type obj struct {
	id  int
	val int
}

func newObjFactory() storage.Item { return &obj{} }

func (o *obj) ID() string     { return strconv.Itoa(o.id) }
func (obj) Namespace() string { return "obj" }

func (o *obj) Marshal() ([]byte, error) {
	buf := make([]byte, 16)
	binary.LittleEndian.PutUint64(buf, uint64(o.id))
	binary.LittleEndian.PutUint64(buf, uint64(o.val))
	return buf, nil
}

func (o *obj) Unmarshal(buf []byte) error {
	if len(buf) != 16 {
		return errors.New("invalid length")
	}
	o.id = int(binary.LittleEndian.Uint64(buf))
	o.val = int(binary.LittleEndian.Uint64(buf))
	return nil
}

func (o *obj) Clone() storage.Item {
	if o == nil {
		return nil
	}
	return &obj{
		id:  o.id,
		val: o.val,
	}
}

func (o obj) String() string {
	return storageutil.JoinFields(o.Namespace(), o.ID())
}
