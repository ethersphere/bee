package migration

import (
	"encoding/binary"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"unsafe"
)

type Version = uint64
type StepFn func(storage.Store) error
type StepsMap = map[Version]StepFn

func Migrate(s storage.Store, sm StepsMap) error {
	version, err := GetVersion(s)
	if err != nil {
		return err
	}
	for key, step := range sm {
		if key == version+1 {
			err := step(s)
			if err != nil {
				return err
			}
			err = s.Put(&storageVersionItem{Version: key})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type storageVersionItem struct {
	Version uint64
}

const storageVersionItemSize = unsafe.Sizeof(&storageVersionItem{})

func (s *storageVersionItem) ID() string {
	return "storage_version"
}

func (s storageVersionItem) Namespace() string {
	return "migration"
}

func (s *storageVersionItem) Marshal() ([]byte, error) {
	buf := make([]byte, storageVersionItemSize)
	binary.LittleEndian.PutUint64(buf, s.Version)
	return buf, nil
}

func (s *storageVersionItem) Unmarshal(bytes []byte) error {
	ni := storageVersionItem{}
	ni.Version = binary.LittleEndian.Uint64(bytes)
	s = &ni
	return nil
}

func GetVersion(s storage.Store) (Version, error) {

	item := storageVersionItem{}
	err := s.Get(&item)
	if err != nil {
		return 0, err
	}
	return item.Version, nil
}
