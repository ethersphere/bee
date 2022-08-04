package storage_test

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/stretchr/testify/assert"
)

const keyPrefix = "test-entity-store"

func TestEntityStore(t *testing.T) {
	store := mock.NewStateStore()

	// This store will map int to ValueType
	s := storage.NewEntityStore(store, keyFromEntityFunc, entityFromKeyFunc, valueUnmarshalFunc)

	// Put some elements to store
	assert.NoError(t, s.Put(1, ValueType{111}))
	assert.NoError(t, s.Put(2, ValueType{222}))

	// Assert interation over keys
	keys := make([]int, 0)
	s.IterateKeys(keyPrefix, func(key interface{}) (stop bool, err error) {
		keys = append(keys, key.(int))
		return false, nil
	})
	assert.ElementsMatch(t, []int{1, 2}, keys)

	// Assert interation over values
	values := make([]ValueType, 0)
	s.IterateValues(keyPrefix, func(value interface{}) (stop bool, err error) {
		values = append(values, value.(ValueType))
		return false, nil
	})
	assert.ElementsMatch(t, []ValueType{{111}, {222}}, values)

	// Assert getting element
	value := &ValueType{}
	assert.NoError(t, s.Get(1, value))
	assert.Equal(t, ValueType{111}, *value)

	// Assert removing element
	assert.NoError(t, s.Delete(1))

	// Assert getting element (should return error)
	assert.EqualError(t, s.Get(1, value), storage.ErrNotFound.Error())
}

func keyFromEntityFunc(key interface{}) string {
	return keyPrefix + strconv.Itoa(key.(int))
}

func entityFromKeyFunc(key string) (interface{}, error) {
	convertedKey, err := strconv.Atoi(key[len(keyPrefix):])
	if err != nil {
		return nil, fmt.Errorf("invalid key: %s, err: %w", key, err)
	}

	return convertedKey, nil
}

func valueUnmarshalFunc(v []byte) (interface{}, error) {
	value := &ValueType{}

	if err := json.Unmarshal(v, value); err != nil {
		return nil, err
	}

	return *value, nil
}

type ValueType struct {
	V int
}
