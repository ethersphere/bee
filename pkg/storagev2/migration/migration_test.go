package migration

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/inmemstore"
)

func TestGetSetVersion(t *testing.T) {
	t.Parallel()

	t.Run("GetVersion", func(t *testing.T) {
		t.Parallel()

		s := inmemstore.New()

		gotVersion, err := GetVersion(s)
		if err != nil {
			t.Errorf("GetVersion should succeed, %v", err)
		}
		if gotVersion != 0 {
			t.Errorf("expect version to be 0, got %v", gotVersion)
		}
	})

	t.Run("SetVersion", func(t *testing.T) {
		t.Parallel()

		s := inmemstore.New()

		const version = 10

		err := SetVersion(s, version)
		if err != nil {
			t.Errorf("SetVersion should succeed, %v", err)
		}

		gotVersion, err := GetVersion(s)
		if err != nil {
			t.Errorf("GetVersion should succeed, %v", err)
		}
		if gotVersion != version {
			t.Errorf("expect version to be %d", version)
		}
	})
}

func TestValidateVersions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   StepsMap
		wantErr bool
	}{
		{
			name: "missing version 3",
			input: StepsMap{
				1: func(s storage.Store) error {
					return s.Put(&obj1{Id: "aaa", SomeInt: 1})
				},
				2: func(s storage.Store) error {
					return s.Put(&obj1{Id: "bbb", SomeInt: 2})
				},
				4: func(s storage.Store) error {
					return s.Put(&obj1{Id: "ccc", SomeInt: 3})
				},
			},
			wantErr: true,
		},
		{
			name: "not missing",
			input: StepsMap{
				1: func(s storage.Store) error {
					return s.Put(&obj1{Id: "aaa", SomeInt: 1})
				},
				2: func(s storage.Store) error {
					return s.Put(&obj1{Id: "bbb", SomeInt: 2})
				},
				3: func(s storage.Store) error {
					return s.Put(&obj1{Id: "ccc", SomeInt: 3})
				},
			},
			wantErr: false,
		},
		{
			name: "desc order versions",
			input: StepsMap{
				3: func(s storage.Store) error {
					return s.Put(&obj1{Id: "aaa", SomeInt: 1})
				},
				2: func(s storage.Store) error {
					return s.Put(&obj1{Id: "bbb", SomeInt: 2})
				},
				1: func(s storage.Store) error {
					return s.Put(&obj1{Id: "ccc", SomeInt: 3})
				},
			},
			wantErr: false,
		},
		{
			name: "desc order version missing",
			input: StepsMap{
				4: func(s storage.Store) error {
					return s.Put(&obj1{Id: "aaa", SomeInt: 1})
				},
				2: func(s storage.Store) error {
					return s.Put(&obj1{Id: "bbb", SomeInt: 2})
				},
				1: func(s storage.Store) error {
					return s.Put(&obj1{Id: "ccc", SomeInt: 3})
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateVersions(tt.input); (err != nil) != tt.wantErr {
				t.Errorf("ValidateVersions() error = %v, wantErr %v", err, tt.wantErr)
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

		steps := StepsMap{
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

		if err := Migrate(s, steps); err != nil {
			t.Errorf("Migrate() error = %v", err)
		}

		newVersion, err := GetVersion(s)
		if err != nil {
			t.Errorf("GetVersion() error = %v", err)
		}
		if newVersion != 3 {
			t.Errorf("new version = %v must be 3", newVersion)
		}

		ObjectExists(t, s, 3, objT1, objT2, objT3)
	})

	t.Run("migration: 5 to 8", func(t *testing.T) {
		t.Parallel()

		steps := StepsMap{
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

		err := SetVersion(s, 5)
		if err != nil {
			t.Errorf("SetVersion() error = %v", err)
		}

		if err := Migrate(s, steps); err != nil {
			t.Errorf("Migrate() error = %v", err)
		}

		newVersion, err := GetVersion(s)
		if err != nil {
			t.Errorf("GetVersion() error = %v", err)
		}
		if newVersion != 8 {
			t.Errorf("new version = %v must be 8", newVersion)
		}

		ObjectExists(t, s, 3, objT1, objT2, objT3)
	})
}
func ObjectExists(t *testing.T, s storage.Store, expectedKeys int, keys ...storage.Key) {
	if len(keys) != expectedKeys {
		t.Errorf("expected %v keys, got %v", expectedKeys, len(keys))
	}
	for _, key := range keys {
		if isExist, _ := s.Has(key); !isExist {
			t.Errorf("key = %v doesn't exists", key)
		}
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
