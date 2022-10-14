package migration

import (
	"encoding/binary"
	"errors"
	"unsafe"

	storage "github.com/ethersphere/bee/pkg/storagev2"
)

type Version = uint64
type StepFn func(storage.Store) error
type StepsMap = map[Version]StepFn

func Migrate(s storage.Store, sm StepsMap) error {
	if err := ValidateVersions(sm); err != nil {
		return err
	}

	currentVersion, err := GetVersion(s)
	if err != nil {
		return err
	}

	for nextVersion := currentVersion + 1; ; nextVersion++ {
		if stepFn, ok := sm[nextVersion]; ok {
			err := stepFn(s)
			if err != nil {
				return err
			}
			err = SetVersion(s, nextVersion)
			if err != nil {
				return err
			}
		} else {
			// there is no next version defiend
			// so we stop here
			return nil
		}
	}
}

func ValidateVersions(sm StepsMap) error {
	// TODO
	// all versions should be in order n (where n min version value), n+1, n+2, n+3... (all values are increasing orders)
	// 5,6,7,8 ok
	// 5,6,7,8,10 nok

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
	s.Version = binary.LittleEndian.Uint64(bytes)
	return nil
}

func GetVersion(s storage.Store) (Version, error) {
	item := storageVersionItem{}
	err := s.Get(&item)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return item.Version, nil
}

func SetVersion(s storage.Store, v Version) error {
	return s.Put(&storageVersionItem{Version: v})
}
