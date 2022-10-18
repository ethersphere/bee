package migration

import (
	"encoding/binary"
	"errors"
	"fmt"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"sort"
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
			// there is no next version defined
			// so we stop here
			return nil
		}
	}
}

// ValidateVersions checks versions if they are in order n (where n min version value), n+1, n+2, n+3... (all values are increasing orders)
func ValidateVersions(sm StepsMap) error {
	versions := make([]int, len(sm))
	i := 0
	for key := range sm {
		versions[i] = int(key)
		i++
	}

	missing := FindMissingNumbers(versions)

	if len(missing) > 0 {
		return fmt.Errorf("missing versions: %v", missing)
	}
	return nil
}

// FindMissingNumbers finds missing numbers in a slice of numbers
func FindMissingNumbers(numbers []int) []int {
	sort.Ints(numbers)
	var missing []int
	for i := 0; i < len(numbers)-1; i++ {
		if numbers[i+1]-numbers[i] != 1 {
			missing = append(missing, numbers[i]+1)
		}
	}
	return missing
}

type storageVersionItem struct {
	Version uint64
}

const storageVersionItemSize = 8

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
