package test

import (
	"encoding/binary"
	"errors"
	"strings"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	leveldbstore "github.com/ethersphere/bee/pkg/storagev2/leveldb"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

type LevelDB struct {
	path  string
	fsync bool
	db    storage.Store
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

func NewLevelDB(path string, fsync bool) (DB, error) {
	db := &LevelDB{
		path:  path,
		fsync: fsync,
	}

	if err := db.init(); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *LevelDB) init() error {
	beeDB, err := leveldbstore.New(db.path, &opt.Options{NoSync: !db.fsync})
	if err != nil {
		return err
	}
	db.db = beeDB
	return nil
}

func (db *LevelDB) Set(key, value []byte) error {
	item := &obj1{
		Id:  string(key),
		Buf: value,
	}

	return db.db.Put(item)
}

func (db *LevelDB) Get(key []byte) ([]byte, error) {
	item := &obj1{
		Id: string(key),
	}

	err := db.db.Get(item)

	switch {
	case err != nil && errors.Is(err, leveldb.ErrNotFound):
		return nil, nil
	case err != nil:
		return nil, err
	}

	return item.Buf, nil
}

func (db *LevelDB) Del(key []byte) error {
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

func (db *LevelDB) Close() error {
	return db.db.Close()
}
