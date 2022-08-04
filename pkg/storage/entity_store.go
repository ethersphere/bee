// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package storage

// EntityStore provides abstraction on top of StateStorer for usecase when
// StateStorer is used for mapping on fixed key to value types.
//
// EntityStore should be improved using generic types once when codebase bumps go version to >=1.18.
type EntityStore interface {
	Get(key interface{}, val interface{}) (err error)
	Put(key interface{}, val interface{}) (err error)
	Delete(key interface{}) (err error)
	IterateKeys(prefix string, iterFunc func(key interface{}) (stop bool, err error)) (err error)
	IterateValues(prefix string, iterFunc func(val interface{}) (stop bool, err error)) (err error)
}

type (
	KeyFromEntityFunc  = func(key interface{}) string
	EntityFromKeyFunc  = func(key string) (interface{}, error)
	ValueUnmarshalFunc = func(v []byte) (interface{}, error)
)

func NewEntityStore(
	store StateStorer,
	toKeyFunc KeyFromEntityFunc,
	fromKeyFunc EntityFromKeyFunc,
	valueUnmarshalFunc ValueUnmarshalFunc,
) EntityStore {
	return &entityStore{
		store:              store,
		toKeyFunc:          toKeyFunc,
		fromKeyFunc:        fromKeyFunc,
		valueUnmarshalFunc: valueUnmarshalFunc,
	}
}

type entityStore struct {
	store              StateStorer
	toKeyFunc          KeyFromEntityFunc
	fromKeyFunc        EntityFromKeyFunc
	valueUnmarshalFunc ValueUnmarshalFunc
}

func (s *entityStore) Get(key interface{}, val interface{}) (err error) {
	return s.store.Get(s.toKeyFunc(key), val)
}

func (s *entityStore) Put(key interface{}, val interface{}) (err error) {
	return s.store.Put(s.toKeyFunc(key), val)
}

func (s *entityStore) Delete(key interface{}) (err error) {
	return s.store.Delete(s.toKeyFunc(key))
}

func (s *entityStore) IterateKeys(prefix string, iterFunc func(key interface{}) (stop bool, err error)) (err error) {
	return s.store.Iterate(prefix, func(rawKey []byte, rawVal []byte) (stop bool, err error) {
		k, err := s.fromKeyFunc(string(rawKey))
		if err != nil {
			return false, nil
		}

		return iterFunc(k)
	})
}

func (s *entityStore) IterateValues(prefix string, iterFunc func(val interface{}) (stop bool, err error)) (err error) {
	return s.store.Iterate(prefix, func(rawKey []byte, rawVal []byte) (stop bool, err error) {
		v, err := s.valueUnmarshalFunc(rawVal)
		if err != nil {
			return false, nil
		}

		return iterFunc(v)
	})
}
