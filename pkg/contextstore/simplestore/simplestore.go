package simplestore

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sync"

	"github.com/boltdb/bolt"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/vmihailenco/msgpack/v5"
)

var ErrNotFound = errors.New("simple store: not found")

type Store struct {
	db *bolt.DB
	wg sync.WaitGroup
}

var chunksBucket = []byte("chunks")

func New(base string) (*Store, error) {
	db, err := bolt.Open(path.Join(base, "data"), 0666, nil)
	if err != nil {
		return nil, err
	}

	err = db.Update(func(txn *bolt.Tx) error {
		_, err := txn.CreateBucketIfNotExists(chunksBucket)
		return err
	})

	if err != nil {
		// todo: cleanup
		return nil, fmt.Errorf("create bucket: %w", err)
	}

	return &Store{
		db: db,
	}, nil
}

func (s *Store) Put(_ context.Context, ch swarm.Chunk) (bool, error) {
	c, err := newChunk(ch)
	exists := false
	if err != nil {
		return false, fmt.Errorf("new chunk: %w", err)
	}
	cbytes, err := c.value()
	if err != nil {
		return false, fmt.Errorf("marshal binary: %w", err)
	}
	err = s.db.Update(func(txn *bolt.Tx) error {
		b := txn.Bucket(chunksBucket)
		exists = b.Get(ch.Address().Bytes()) == nil
		return b.Put(ch.Address().Bytes(), cbytes)
	})
	return exists, err
}

func (s *Store) Get(_ context.Context, address swarm.Address) (swarm.Chunk, error) {
	var ch swarm.Chunk
	err := s.db.View(func(txn *bolt.Tx) error {
		b := txn.Bucket(chunksBucket)
		cbytes := b.Get(address.Bytes())
		if cbytes == nil {
			return storage.ErrNotFound
		}
		c := new(chunk)
		err := c.unmarshal(cbytes)
		if err != nil {
			return fmt.Errorf("unmarshal chunk: %w", err)
		}
		stamp := new(postage.Stamp)
		if err = stamp.UnmarshalBinary(c.Stamp); err != nil {
			return fmt.Errorf("unmarshal stamp: %w", err)
		}

		ch = swarm.NewChunk(swarm.NewAddress(c.Address), c.Data).WithStamp(stamp)
		return nil
	})

	return ch, err
}

func (s *Store) Iterate(f storage.IterateChunkFn) error {
	return s.db.View(func(txn *bolt.Tx) error {
		c := txn.Bucket(chunksBucket).Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			ch := new(chunk)
			err := ch.unmarshal(v)
			if err != nil {
				return fmt.Errorf("unmarshal chunk: %w", err)
			}
			stamp := new(postage.Stamp)
			if err = stamp.UnmarshalBinary(ch.Stamp); err != nil {
				return fmt.Errorf("unmarshal stamp: %w", err)
			}

			chunk := swarm.NewChunk(swarm.NewAddress(ch.Address), ch.Data).WithStamp(stamp)
			stop, err := f(chunk)
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
		}
		return nil
	})
}

func (s *Store) Has(_ context.Context, address swarm.Address) (bool, error) {
	err := s.db.View(func(txn *bolt.Tx) error {
		b := txn.Bucket(chunksBucket)
		cbytes := b.Get(address.Bytes())
		if cbytes == nil {
			return storage.ErrNotFound
		}
		return nil
	})

	if errors.Is(err, storage.ErrNotFound) {
		return false, nil
	}

	return err == nil, err
}

// Count returns the number of stored chunks in
// the Store.
func (s *Store) Count() (n int, err error) {
	err = s.db.View(func(txn *bolt.Tx) error {
		b := txn.Bucket(chunksBucket)
		n = b.Stats().KeyN
		return nil
	})

	return n, err
}

// Delete the chunk with the given address.
func (s *Store) Delete(_ context.Context, addr swarm.Address) error {
	return s.db.Update(func(txn *bolt.Tx) error {
		b := txn.Bucket(chunksBucket)
		return b.Delete(addr.Bytes())
	})
}

func (s *Store) Protect() func() {
	s.wg.Add(1)
	return func() { s.wg.Done() }
}

// Close closes the underlying boltDB.
func (s *Store) Close() (err error) {
	s.wg.Wait()
	return s.db.Close()
}

type chunk struct {
	Address []byte `msgpack:"address"`
	Data    []byte `msgpack:"data"`
	Stamp   []byte `msgpack:"stamp"`
}

func newChunk(ch swarm.Chunk) (*chunk, error) {
	var (
		err error
		c   = new(chunk)
	)
	c.Address = ch.Address().Bytes()
	c.Data = ch.Data()
	c.Stamp, err = ch.Stamp().MarshalBinary()
	return c, err
}

func (c *chunk) value() ([]byte, error) {
	return msgpack.Marshal(c)
}

func (c *chunk) unmarshal(b []byte) error {
	return msgpack.Unmarshal(b, &c)
}
