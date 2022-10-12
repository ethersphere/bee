package migration

import (
	"encoding/binary"
	"errors"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	"strings"
	"testing"
)

func TestMigrate(t *testing.T) {
	t.Parallel()

	S := inmemstore.New()
	stepsMapTests := StepsMap{
		1: func(s storage.Store) error {
			err := s.Put(&obj1{
				Id:      "aaaaaaaaaaa",
				SomeInt: 3,
				Buf:     []byte("hello"),
			})
			if err != nil {
				return err
			}
			return nil
		},
		2: func(s storage.Store) error {
			err := s.Put(&obj1{
				Id:      "bbbbbbbbbbb",
				SomeInt: 1,
				Buf:     []byte("hey"),
			})
			if err != nil {
				return err
			}
			return nil
		},
	}

	t.Run("migration test", func(t *testing.T) {
		t.Parallel()
		if err := Migrate(S, stepsMapTests); err != nil {
			t.Errorf("Migrate() error = %v", err)
		}
	})
}

type obj1 struct {
	Id      string
	SomeInt uint64
	Buf     []byte
}

func (obj1) Namespace() string { return "obj1" }

func (o *obj1) ID() string { return o.Id }

// ID is 32 bytes
func (o *obj1) Marshal() ([]byte, error) {
	buf := make([]byte, 40)
	copy(buf[:32], []byte(o.Id))
	binary.LittleEndian.PutUint64(buf[32:], o.SomeInt)
	buf = append(buf, o.Buf[:]...)
	return buf, nil
}

func (o *obj1) Unmarshal(buf []byte) error {
	if len(buf) < 40 {
		return errors.New("invalid length")
	}
	o.Id = strings.TrimRight(string(buf[:32]), string([]byte{0}))
	o.SomeInt = binary.LittleEndian.Uint64(buf[32:])
	o.Buf = buf[40:]
	return nil
}
