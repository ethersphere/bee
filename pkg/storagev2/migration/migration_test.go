package migration

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/inmemstore"
)

func TestMigrate(t *testing.T) {
	t.Parallel()

	S := inmemstore.New()
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

	t.Run("migration test", func(t *testing.T) {
		t.Parallel()
		if err := Migrate(S, stepsMapTests); err != nil {
			t.Errorf("Migrate() error = %v", err)
		}
	})
}

func Test_GetVersion(t *testing.T) {
	t.Parallel()

	t.Run("default version", func(t *testing.T) {
		t.Parallel()

		s := inmemstore.New()
		v, err := GetVersion(s)
		if err != nil {
			t.Errorf("GetVersion should succeed:  %v", err)
		}

		if v != 0 {
			t.Error("version must be zero")
		}
	})

	t.Run("with version", func(t *testing.T) {
		t.Parallel()

		const version = 10

		s := inmemstore.New()
		err := SetVersion(s, version)
		if err != nil {
			t.Errorf("SetVersion should succeed:  %v", err)
		}

		v, err := GetVersion(s)
		if err != nil {
			t.Errorf("GetVersion should succeed:  %v", err)
		}

		if v != version {
			t.Errorf("have %v, want %v", v, version)
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
