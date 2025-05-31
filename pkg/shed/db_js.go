//go:build js
// +build js

package shed

import (
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
)

// DB provides abstractions over LevelDB in order to
// implement complex structures using fields and ordered indexes.
// It provides a schema functionality to store fields and indexes
// information about naming and types.
type DB struct {
	ldb  *leveldb.DB
	quit chan struct{} // Quit channel to stop the metrics collection before closing the database
}

// NewDBWrap returns new DB which uses the given ldb as its underlying storage.
// The function will panics if the given ldb is nil.
func NewDBWrap(ldb *leveldb.DB) (db *DB, err error) {
	if ldb == nil {
		panic(errors.New("shed: NewDBWrap: nil ldb"))
	}

	db = &DB{
		ldb: ldb,
	}

	if _, err = db.getSchema(); err != nil {
		if errors.Is(err, leveldb.ErrNotFound) {
			// Save schema with initialized default fields.
			if err = db.putSchema(schema{
				Fields:  make(map[string]fieldSpec),
				Indexes: make(map[byte]indexSpec),
			}); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	// Create a quit channel for the periodic metrics collector and run it.
	db.quit = make(chan struct{})

	return db, nil
}

// Put wraps LevelDB Put method to increment metrics counter.
func (db *DB) Put(key, value []byte) (err error) {
	err = db.ldb.Put(key, value, nil)
	if err != nil {
		return err
	}
	return nil
}

// Get wraps LevelDB Get method to increment metrics counter.
func (db *DB) Get(key []byte) (value []byte, err error) {
	value, err = db.ldb.Get(key, nil)
	if err != nil {
		return nil, err
	}
	return value, nil
}

// Has wraps LevelDB Has method to increment metrics counter.
func (db *DB) Has(key []byte) (yes bool, err error) {
	yes, err = db.ldb.Has(key, nil)
	if err != nil {
		return false, err
	}
	return yes, nil
}

// Delete wraps LevelDB Delete method to increment metrics counter.
func (db *DB) Delete(key []byte) (err error) {
	err = db.ldb.Delete(key, nil)
	if err != nil {
		return err
	}
	return nil
}

// NewIterator wraps LevelDB NewIterator method to increment metrics counter.
func (db *DB) NewIterator() iterator.Iterator {
	return db.ldb.NewIterator(nil, nil)
}

// WriteBatch wraps LevelDB Write method to increment metrics counter.
func (db *DB) WriteBatch(batch *leveldb.Batch) (err error) {
	err = db.ldb.Write(batch, nil)
	if err != nil {
		return err
	}
	return nil
}
