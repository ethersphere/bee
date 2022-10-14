package migration

import (
	"encoding/binary"
	"errors"
	"fmt"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	"strings"
	"testing"
)

func TestMigrate(t *testing.T) {
	t.Parallel()

	S := inmemstore.New()
	currentVersion, _ := GetVersion(S)
	fmt.Println("currentVersion", currentVersion)
	stepsMapTests := StepsMap{
		1: func(s storage.Store) error {
			err := s.Put(&obj1{
				Id:      "aaaaaaaaaaa",
				SomeInt: 3,
			})
			return err
		},
		2: func(s storage.Store) error {
			err := s.Put(&obj1{
				Id:      "bbbbbbbbbbb",
				SomeInt: 1,
			})
			return err
		},
	}

	// initial store version (e.g. before migration it should be 0, after migration it should be 2)
	t.Run("migration test", func(t *testing.T) {
		t.Parallel()
		if currentVersion != 0 {
			t.Errorf("current version = %v should be zero", currentVersion)
		}
		if err := Migrate(S, stepsMapTests); err != nil {
			t.Errorf("Migrate() error = %v", err)
		}
		newVersion, err := GetVersion(S)

		if err != nil {
			t.Errorf("GetVersion() error = %v", err)
		}
		if currentVersion >= newVersion {
			t.Errorf("new version = %v must be greater than current version = %v", currentVersion, newVersion)
		}
	})

	currentVersion, _ = GetVersion(S)
	S.Put(&obj1{
		Id:      "aaaaaaaaaaa",
		SomeInt: 3,
	})
	t.Run("migration with store items", func(t *testing.T) {
		t.Parallel()

		if currentVersion != 0 {
			t.Errorf("current version = %v should be zero", currentVersion)
		}
		if err := Migrate(S, stepsMapTests); err != nil {
			t.Errorf("Migrate() error = %v", err)
		}

		newVersion, err := GetVersion(S)
		fmt.Println("newVersion--", newVersion)
		if err != nil {
			t.Errorf("GetVersion() error = %v", err)
		}
		if currentVersion >= newVersion {
			t.Errorf("new version = %v must be greater than current version = %v", currentVersion, newVersion)
		}
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
