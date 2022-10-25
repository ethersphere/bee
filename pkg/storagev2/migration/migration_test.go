// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"encoding/binary"
	"errors"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	"github.com/ethersphere/bee/pkg/storagev2/migration"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
	"math"
	"math/rand"
	"strings"
	"testing"
)

var (
	stepErr = errors.New("step error")
)

func TestGetSetVersion(t *testing.T) {
	t.Parallel()

	t.Run("Version", func(t *testing.T) {
		t.Parallel()

		s := inmemstore.New()

		gotVersion, err := migration.Version(s)
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

		err := migration.SetVersion(s, version)
		if err != nil {
			t.Errorf("SetVersion() unexpected error: %v", err)
		}

		gotVersion, err := migration.Version(s)
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
	objT1 := &obj1{Id: "aaa", SomeInt: 1}
	objT2 := &obj1{Id: "bbb", SomeInt: 2}
	objT3 := &obj1{Id: "ccc", SomeInt: 3}

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
				1: func(s storage.Store) error {
					return s.Put(objT1)
				},
				2: func(s storage.Store) error {
					return s.Put(objT2)
				},
				4: func(s storage.Store) error {
					return s.Put(objT3)
				},
			},
			wantErr: true,
		},
		{
			name: "not missing",
			input: migration.Steps{
				1: func(s storage.Store) error {
					return s.Put(objT1)
				},
				2: func(s storage.Store) error {
					return s.Put(objT2)
				},
				3: func(s storage.Store) error {
					return s.Put(objT3)
				},
			},
			wantErr: false,
		},
		{
			name: "desc order versions",
			input: migration.Steps{
				3: func(s storage.Store) error {
					return s.Put(objT1)
				},
				2: func(s storage.Store) error {
					return s.Put(objT2)
				},
				1: func(s storage.Store) error {
					return s.Put(objT3)
				},
			},
			wantErr: false,
		},
		{
			name: "desc order version missing",
			input: migration.Steps{
				4: func(s storage.Store) error {
					return s.Put(objT1)
				},
				2: func(s storage.Store) error {
					return s.Put(objT2)
				},
				1: func(s storage.Store) error {
					return s.Put(objT3)
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
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
	objT1 := &obj1{Id: "aaa", SomeInt: 1}
	objT2 := &obj1{Id: "bbb", SomeInt: 2}
	objT3 := &obj1{Id: "ccc", SomeInt: 3}

	t.Run("migration: 0 to 3", func(t *testing.T) {
		t.Parallel()

		steps := migration.Steps{
			1: func(s storage.Store) error {
				return s.Put(objT1)
			},
			2: func(s storage.Store) error {
				return s.Put(objT2)
			},
			3: func(s storage.Store) error {
				return s.Put(objT3)
			},
		}

		s := inmemstore.New()

		if err := migration.Migrate(s, steps); err != nil {
			t.Errorf("Migrate() unexpected error: %v", err)
		}

		newVersion, err := migration.Version(s)
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

		steps := migration.Steps{
			8: func(s storage.Store) error {
				return s.Put(objT1)
			},
			7: func(s storage.Store) error {
				return s.Put(objT2)
			},
			6: func(s storage.Store) error {
				return s.Put(objT3)
			},
		}

		s := inmemstore.New()

		err := migration.SetVersion(s, 5)
		if err != nil {
			t.Errorf("SetVersion() unexpected error: %v", err)
		}

		if err := migration.Migrate(s, steps); err != nil {
			t.Errorf("Migrate() unexpected error: %v", err)
		}

		newVersion, err := migration.Version(s)
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

		steps := migration.Steps{
			8: func(s storage.Store) error {
				return s.Put(objT1)
			},
			7: func(s storage.Store) error {
				return stepErr
			},
			6: func(s storage.Store) error {
				return s.Put(objT3)
			},
		}

		s := inmemstore.New()

		err := migration.SetVersion(s, 5)
		if err != nil {
			t.Errorf("SetVersion() unexpected error: %v", err)
		}

		if err := migration.Migrate(s, steps); err != nil && !errors.Is(err, stepErr) {
			t.Errorf("Migrate() unexpected error: %v", err)
		}

		newVersion, err := migration.Version(s)
		if err != nil {
			t.Errorf("Version() unexpected error: %v", err)
		}
		// since we have error on step 7, we should be on version 6
		if newVersion != 6 {
			t.Errorf("new version = %v must be 6", newVersion)
		}
	})
}
func assertObjectExists(t *testing.T, s storage.Store, keys ...storage.Key) {
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
		}}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			storagetest.TestItemMarshalAndUnmarshal(t, tc.test)
		})
	}
}

type obj1 struct {
	Id      string
	SomeInt uint64
}

func (obj1) Namespace() string { return "obj1" }

func (o *obj1) ID() string { return o.Id }

// ID is 32 bytes
func (o *obj1) Marshal() ([]byte, error) {
	buf := make([]byte, 40)
	copy(buf[:32], []byte(o.Id))
	binary.LittleEndian.PutUint64(buf[32:], o.SomeInt)
	return buf, nil
}

func (o *obj1) Unmarshal(buf []byte) error {
	if len(buf) < 40 {
		return errors.New("invalid length")
	}
	o.Id = strings.TrimRight(string(buf[:32]), string([]byte{0}))
	o.SomeInt = binary.LittleEndian.Uint64(buf[32:])
	return nil
}
