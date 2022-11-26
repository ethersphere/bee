package bolt

import (
	"bytes"
	"fmt"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/hashicorp/go-multierror"
	bolt "go.etcd.io/bbolt"
)

type Store struct {
	db *bolt.DB
}

type Tx struct {
	tx    *bolt.Tx
	write bool
}

var _ storage.Storage = (*Tx)(nil)

func New(path string, buckets ...string) (*Store, error) {

	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, err
	}

	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range buckets {
			_, err := tx.CreateBucketIfNotExists([]byte(b))
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &Store{
		db: db,
	}, nil
}

func (s *Store) Close() (err error) {
	return s.db.Close()
}

func (s *Store) NewTransaction(write bool) (*Tx, error) {
	tx, err := s.db.Begin(write)
	if err != nil {
		return nil, err
	}

	return &Tx{tx, write}, nil
}

func (t *Tx) Get(item storage.Item) error {
	b := t.tx.Bucket([]byte(item.Namespace()))
	v := b.Get([]byte(item.ID()))
	return item.Unmarshal(v)
}

// Has implements the storage.Store interface.
func (t *Tx) Has(k storage.Key) (has bool, err error) {
	b := t.tx.Bucket([]byte(k.Namespace()))
	v := b.Get([]byte(k.ID()))
	if v != nil {
		has = true
	}
	return
}

// GetSize implements the storage.Store interface.
func (t *Tx) GetSize(k storage.Key) (int, error) {
	return 0, nil
}

func (t *Tx) Put(item storage.Item) error {

	value, err := item.Marshal()
	if err != nil {
		return fmt.Errorf("failed serializing: %w", err)
	}

	b := t.tx.Bucket([]byte(item.Namespace()))
	return b.Put([]byte(item.ID()), value)
}

func (t *Tx) Delete(item storage.Item) error {
	b := t.tx.Bucket([]byte(item.Namespace()))
	return b.Delete([]byte(item.ID()))
}

func (t *Tx) Commit() error {

	if t.write {
		return t.tx.Commit()
	}

	// Rollback must be called for read-only transaction
	return t.tx.Rollback()
}

func (t *Tx) Iterate(q storage.Query, fn storage.IterateFn) error {

	if err := q.Validate(); err != nil {
		return fmt.Errorf("failed iteration: %w", err)
	}

	c := t.tx.Bucket([]byte(q.Factory().Namespace())).Cursor()

	var retErr *multierror.Error

	if q.Prefix != nil {
		for k, v := c.Seek(q.Prefix); k != nil && bytes.HasPrefix(k, q.Prefix); k, v = c.Next() {

			var err error
			var res *storage.Result

			switch q.ItemAttribute {
			case storage.QueryItemID, storage.QueryItemSize:
				res = &storage.Result{ID: string(k), Size: len(v)}
			case storage.QueryItem:
				newItem := q.Factory()
				err = newItem.Unmarshal(v)
				res = &storage.Result{Entry: newItem}
			}

			if err != nil {
				retErr = multierror.Append(retErr, fmt.Errorf("failed unmarshaling: %w", err))
				break
			}

			if res == nil {
				retErr = multierror.Append(retErr, fmt.Errorf("unknown object attribute type: %v", q.ItemAttribute))
				break
			}

			if stop, err := fn(*res); err != nil {
				retErr = multierror.Append(retErr, fmt.Errorf("iterate callback function errored: %w", err))
				break
			} else if stop {
				break
			}
		}
	}

	return retErr.ErrorOrNil()
}

func (t *Tx) Count(key storage.Key) (count int, err error) {
	b := t.tx.Bucket([]byte(key.Namespace()))
	err = b.ForEach(func(k, v []byte) error {
		count++
		return nil
	})
	return
}
