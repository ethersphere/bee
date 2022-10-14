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

	t.Run("GetVersion v1", func(t *testing.T) {
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

	t.Run("GetVersion v2", func(t *testing.T) {
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

	// TODO
}

func TestMigrate(t *testing.T) {
	t.Parallel()

	t.Run("migration: 0 to 3", func(t *testing.T) {
		t.Parallel()

		steps := StepsMap{
			1: func(s storage.Store) error {
				return s.Put(&obj1{Id: "aaa", SomeInt: 1})
			},
			2: func(s storage.Store) error {
				return s.Put(&obj1{Id: "bbb", SomeInt: 2})
			},
			3: func(s storage.Store) error {
				return s.Put(&obj1{Id: "ccc", SomeInt: 3})
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

		// TODO Assert that migrated values are in store
	})

	t.Run("migration: 5 to 8", func(t *testing.T) {
		t.Parallel()

		steps := StepsMap{
			8: func(s storage.Store) error {
				return s.Put(&obj1{Id: "aaa", SomeInt: 1})
			},
			7: func(s storage.Store) error {
				return s.Put(&obj1{Id: "bbb", SomeInt: 2})
			},
			6: func(s storage.Store) error {
				return s.Put(&obj1{Id: "ccc", SomeInt: 3})
			},
		}

		s := inmemstore.New()

		SetVersion(s, 5)

		if err := Migrate(s, steps); err != nil {
			t.Errorf("Migrate() error = %v", err)
		}

		newVersion, err := GetVersion(s)
		if err != nil {
			t.Errorf("GetVersion() error = %v", err)
		}
		if newVersion != 8 {
			t.Errorf("new version = %v must be 3", newVersion)
		}

		// TODO Assert that migrated values are in store
	})
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
