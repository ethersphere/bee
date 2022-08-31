package test

import (
	"errors"
	"sync"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	inmem "github.com/ethersphere/bee/pkg/storagev2/inmemstore"
)

type InMem struct {
	db storage.Store
	mu sync.RWMutex
}

func NewInMem() (DB, error) {
	db := new(InMem)

	beeDB := inmem.New()
	db.db = beeDB

	return db, nil
}

func (db *InMem) Set(key, value []byte) error {
	db.mu.RLock()
	defer db.mu.RUnlock()

	item := &obj1{
		Id:  string(key),
		Buf: value,
	}

	return db.db.Put(item)
}

func (db *InMem) Get(key []byte) ([]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	item := &obj1{
		Id: string(key),
	}

	err := db.db.Get(item)

	switch {
	case err != nil && errors.Is(err, storage.ErrNotFound):
		return nil, nil
	case err != nil:
		return nil, err
	}

	return item.Buf, nil
}

func (db *InMem) Del(key []byte) error {
	db.mu.RLock()
	defer db.mu.RUnlock()

	item := &obj1{
		Id: string(key),
	}

	ok, err := db.db.Has(item)
	if !ok || err != nil {
		return err
	}

	err = db.db.Delete(item)
	if err != nil {
		return err
	}

	return nil
}

func (db *InMem) Close() error {
	db.mu.RLock()
	defer db.mu.RUnlock()

	return db.db.Close()
}
