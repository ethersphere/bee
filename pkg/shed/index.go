// Copyright 2018 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package shed

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
)

// Item holds fields relevant to Swarm Chunk data and metadata.
// All information required for swarm storage and operations
// on that storage must be defined here.
// This structure is logically connected to swarm storage,
// the only part of this package that is not generalized,
// mostly for performance reasons.
//
// Item is a type that is used for retrieving, storing and encoding
// chunk data and metadata. It is passed as an argument to Index encoding
// functions, get function and put function.
// But it is also returned with additional data from get function call
// and as the argument in iterator function definition.
type Item struct {
	Address         []byte
	Data            []byte
	AccessTimestamp int64
	StoreTimestamp  int64
	BinID           uint64
	PinCounter      uint64 // maintains the no of time a chunk is pinned
	Tag             uint32
	BatchID         []byte  // postage batch ID
	Sig             []byte  // postage stamp
	Radius          uint8   // postage batch reserve radius, po upto and excluding which chunks are unpinned
	Counts          *Counts // postage batch counts
}

type Counts struct {
	Counts []byte
}

func (c *Counts) String() string {
	counts := make([]uint32, len(c.Counts)/4)
	for i := range counts {
		counts[i] = c.add(i, 0, true)
	}
	return fmt.Sprintf("%v", counts)
}

func (c *Counts) Inc(i int) uint32 {
	return c.add(i, 1, true)
}
func (c *Counts) Dec(i int) uint32 {
	return c.add(i, 1, false)
}

func (c *Counts) add(i int, d uint32, add bool) uint32 {
	count := binary.BigEndian.Uint32(c.Counts[4*i : 4*i+4]) // deserialise
	if add {
		count += d
	} else {
		count -= d
	}
	binary.BigEndian.PutUint32(c.Counts[4*i:4*i+4], count)
	return count
}

// Merge is a helper method to construct a new
// Item by filling up fields with default values
// of a particular Item with values from another one.
func (i Item) Merge(i2 Item) Item {
	if i.Address == nil {
		i.Address = i2.Address
	}
	if i.Data == nil {
		i.Data = i2.Data
	}
	if i.AccessTimestamp == 0 {
		i.AccessTimestamp = i2.AccessTimestamp
	}
	if i.StoreTimestamp == 0 {
		i.StoreTimestamp = i2.StoreTimestamp
	}
	if i.BinID == 0 {
		i.BinID = i2.BinID
	}
	if i.PinCounter == 0 {
		i.PinCounter = i2.PinCounter
	}
	if i.Tag == 0 {
		i.Tag = i2.Tag
	}
	if i.Sig == nil {
		i.Sig = i2.Sig
	}
	if i.BatchID == nil {
		i.BatchID = i2.BatchID
	}
	if i.Radius == 0 {
		i.Radius = i2.Radius
	}
	if i.Counts == nil {
		i.Counts = i2.Counts
	}
	return i
}

// Index represents a set of LevelDB key value pairs that have common
// prefix. It holds functions for encoding and decoding keys and values
// to provide transparent actions on saved data which inclide:
// - getting a particular Item
// - saving a particular Item
// - iterating over a sorted LevelDB keys
// It implements IndexIteratorInterface interface.
type Index struct {
	db              *DB
	prefix          []byte
	encodeKeyFunc   func(fields Item) (key []byte, err error)
	decodeKeyFunc   func(key []byte) (e Item, err error)
	encodeValueFunc func(fields Item) (value []byte, err error)
	decodeValueFunc func(keyFields Item, value []byte) (e Item, err error)
}

// IndexFuncs structure defines functions for encoding and decoding
// LevelDB keys and values for a specific index.
type IndexFuncs struct {
	EncodeKey   func(fields Item) (key []byte, err error)
	DecodeKey   func(key []byte) (e Item, err error)
	EncodeValue func(fields Item) (value []byte, err error)
	DecodeValue func(keyFields Item, value []byte) (e Item, err error)
}

// NewIndex returns a new Index instance with defined name and
// encoding functions. The name must be unique and will be validated
// on database schema for a key prefix byte.
func (db *DB) NewIndex(name string, funcs IndexFuncs) (f Index, err error) {
	id, err := db.schemaIndexPrefix(name)
	if err != nil {
		return f, fmt.Errorf("get schema index prefix: %w", err)
	}
	prefix := []byte{id}
	return Index{
		db:     db,
		prefix: prefix,
		// This function adjusts Index LevelDB key
		// by appending the provided index id byte.
		// This is needed to avoid collisions between keys of different
		// indexes as all index ids are unique.
		encodeKeyFunc: func(e Item) (key []byte, err error) {
			key, err = funcs.EncodeKey(e)
			if err != nil {
				return nil, err
			}
			return append(append(make([]byte, 0, len(key)+1), prefix...), key...), nil
		},
		// This function reverses the encodeKeyFunc constructed key
		// to transparently work with index keys without their index ids.
		// It assumes that index keys are prefixed with only one byte.
		decodeKeyFunc: func(key []byte) (e Item, err error) {
			return funcs.DecodeKey(key[1:])
		},
		encodeValueFunc: funcs.EncodeValue,
		decodeValueFunc: funcs.DecodeValue,
	}, nil
}

// ItemKey accepts an Item and returns generated key for it.
func (f Index) ItemKey(item Item) (key []byte, err error) {
	return f.encodeKeyFunc(item)
}

// Get accepts key fields represented as Item to retrieve a
// value from the index and return maximum available information
// from the index represented as another Item.
func (f Index) Get(keyFields Item) (out Item, err error) {
	key, err := f.encodeKeyFunc(keyFields)
	if err != nil {
		return out, fmt.Errorf("encode key: %w", err)
	}
	value, err := f.db.Get(key)
	if err != nil {
		return out, fmt.Errorf("get value: %w", err)
	}
	out, err = f.decodeValueFunc(keyFields, value)
	if err != nil {
		return out, fmt.Errorf("decode value: %w", err)
	}
	return out.Merge(keyFields), nil
}

// Fill populates fields on provided items that are part of the
// encoded value by getting them based on information passed in their
// fields. Every item must have all fields needed for encoding the
// key set. The passed slice items will be changed so that they
// contain data from the index values. No new slice is allocated.
// This function uses a single leveldb snapshot.
func (f Index) Fill(items []Item) (err error) {
	snapshot, err := f.db.ldb.GetSnapshot()
	if err != nil {
		return fmt.Errorf("get snapshot: %w", err)
	}
	defer snapshot.Release()

	for i, item := range items {
		key, err := f.encodeKeyFunc(item)
		if err != nil {
			return fmt.Errorf("encode key: %w", err)
		}
		value, err := snapshot.Get(key, nil)
		if err != nil {
			return fmt.Errorf("get value: %w", err)
		}
		v, err := f.decodeValueFunc(item, value)
		if err != nil {
			return fmt.Errorf("decode value: %w", err)
		}
		items[i] = v.Merge(item)
		// items[i] = item.Merge(v)
	}
	return nil
}

// Has accepts key fields represented as Item to check
// if there this Item's encoded key is stored in
// the index.
func (f Index) Has(keyFields Item) (bool, error) {
	key, err := f.encodeKeyFunc(keyFields)
	if err != nil {
		return false, fmt.Errorf("encode key: %w", err)
	}
	return f.db.Has(key)
}

// HasMulti accepts multiple multiple key fields represented as Item to check if
// there this Item's encoded key is stored in the index for each of them.
func (f Index) HasMulti(items ...Item) ([]bool, error) {
	have := make([]bool, len(items))
	snapshot, err := f.db.ldb.GetSnapshot()
	if err != nil {
		return nil, fmt.Errorf("get snapshot: %w", err)
	}
	defer snapshot.Release()
	for i, keyFields := range items {
		key, err := f.encodeKeyFunc(keyFields)
		if err != nil {
			return nil, fmt.Errorf("encode key for address %x: %w", keyFields.Address, err)
		}
		have[i], err = snapshot.Has(key, nil)
		if err != nil {
			return nil, fmt.Errorf("has key for address %x: %w", keyFields.Address, err)
		}
	}
	return have, nil
}

// Put accepts Item to encode information from it
// and save it to the database.
func (f Index) Put(i Item) (err error) {
	key, err := f.encodeKeyFunc(i)
	if err != nil {
		return fmt.Errorf("encode key: %w", err)
	}
	value, err := f.encodeValueFunc(i)
	if err != nil {
		return fmt.Errorf("encode value: %w", err)
	}
	return f.db.Put(key, value)
}

// PutInBatch is the same as Put method, but it just
// saves the key/value pair to the batch instead
// directly to the database.
func (f Index) PutInBatch(batch *leveldb.Batch, i Item) (err error) {
	key, err := f.encodeKeyFunc(i)
	if err != nil {
		return fmt.Errorf("encode key: %w", err)
	}
	value, err := f.encodeValueFunc(i)
	if err != nil {
		return fmt.Errorf("encode value: %w", err)
	}
	batch.Put(key, value)
	return nil
}

// Delete accepts Item to remove a key/value pair
// from the database based on its fields.
func (f Index) Delete(keyFields Item) (err error) {
	key, err := f.encodeKeyFunc(keyFields)
	if err != nil {
		return fmt.Errorf("encode key: %w", err)
	}
	return f.db.Delete(key)
}

// DeleteInBatch is the same as Delete just the operation
// is performed on the batch instead on the database.
func (f Index) DeleteInBatch(batch *leveldb.Batch, keyFields Item) (err error) {
	key, err := f.encodeKeyFunc(keyFields)
	if err != nil {
		return fmt.Errorf("encode key: %w", err)
	}
	batch.Delete(key)
	return nil
}

// IndexIterFunc is a callback on every Item that is decoded
// by iterating on an Index keys.
// By returning a true for stop variable, iteration will
// stop, and by returning the error, that error will be
// propagated to the called iterator method on Index.
type IndexIterFunc func(item Item) (stop bool, err error)

// IterateOptions defines optional parameters for Iterate function.
type IterateOptions struct {
	// StartFrom is the Item to start the iteration from.
	StartFrom *Item
	// If SkipStartFromItem is true, StartFrom item will not
	// be iterated on.
	SkipStartFromItem bool
	// Iterate over items which keys have a common prefix.
	Prefix []byte
	// Iterate over items in reverse order.
	Reverse bool
}

// Iterate function iterates over keys of the Index.
// If IterateOptions is nil, the iterations is over all keys.
func (f Index) Iterate(fn IndexIterFunc, options *IterateOptions) (err error) {
	if options == nil {
		options = new(IterateOptions)
	}
	// construct a prefix with Index prefix and optional common key prefix
	prefix := append(f.prefix, options.Prefix...)
	// start from the prefix
	startKey := prefix
	if options.StartFrom != nil {
		// start from the provided StartFrom Item key value
		startKey, err = f.encodeKeyFunc(*options.StartFrom)
		if err != nil {
			return fmt.Errorf("encode key: %w", err)
		}
	}

	it := f.db.NewIterator()
	defer it.Release()

	var ok bool

	// move the cursor to the start key
	ok = it.Seek(startKey)

	if !options.Reverse {
		if !ok {
			// stop iterator if seek has failed
			return it.Error()
		}
	} else {
		// reverse seeker
		if options.StartFrom != nil {
			if !ok {
				return it.Error()
			}
		} else {
			// find last key for this index (and prefix)

			// move cursor to last key
			ok = it.Last()
			if !ok {
				return it.Error()
			}

			if lastKeyHasPrefix := bytes.HasPrefix(it.Key(), prefix); !lastKeyHasPrefix {
				// increment last prefix byte (that is not 0xFF) to try to find last key
				incrementedPrefix := bytesIncrement(prefix)
				if incrementedPrefix == nil {
					return fmt.Errorf("index iterator invalid prefix: %v -> %v", prefix, string(prefix))
				}

				// should find first key after prefix (same or different index)
				ok = it.Seek(incrementedPrefix)
				if !ok {
					return it.Error()
				}

				// previous key should have proper prefix
				ok = it.Prev()
				if !ok {
					return it.Error()
				}
			}
		}
	}

	itSeekerFn := it.Next
	if options.Reverse {
		itSeekerFn = it.Prev
	}
	if options.SkipStartFromItem && bytes.Equal(startKey, it.Key()) {
		// skip the start from Item if it is the first key
		// and it is explicitly configured to skip it
		ok = itSeekerFn()
	}
	for ; ok; ok = itSeekerFn() {
		item, err := f.itemFromIterator(it, prefix)
		if err != nil {
			if errors.Is(err, leveldb.ErrNotFound) {
				break
			}
			return fmt.Errorf("get item from iterator: %w", err)
		}
		stop, err := fn(item)
		if err != nil {
			return fmt.Errorf("index iterator function: %w", err)
		}
		if stop {
			break
		}
	}
	return it.Error()
}

// bytesIncrement increments the last byte that is not 0xFF, and returns
// a new byte array truncated after the position that was incremented.
func bytesIncrement(bytes []byte) []byte {
	b := append(bytes[:0:0], bytes...)

	for i := len(bytes) - 1; i >= 0; {
		if b[i] == 0xFF {
			i--
			continue
		}

		// found byte smaller than 0xFF: increment and truncate
		b[i]++
		return b[:i+1]
	}

	// input contained only 0xFF bytes
	return nil
}

// First returns the first item in the Index which encoded key starts with a prefix.
// If the prefix is nil, the first element of the whole index is returned.
// If Index has no elements, a leveldb.ErrNotFound error is returned.
func (f Index) First(prefix []byte) (i Item, err error) {
	it := f.db.NewIterator()
	defer it.Release()

	totalPrefix := append(f.prefix, prefix...)
	it.Seek(totalPrefix)

	return f.itemFromIterator(it, totalPrefix)
}

// itemFromIterator returns the Item from the current iterator position.
// If the complete encoded key does not start with totalPrefix,
// leveldb.ErrNotFound is returned. Value for totalPrefix must start with
// Index prefix.
func (f Index) itemFromIterator(it iterator.Iterator, totalPrefix []byte) (i Item, err error) {
	key := it.Key()
	if !bytes.HasPrefix(key, totalPrefix) {
		return i, leveldb.ErrNotFound
	}
	// create a copy of key byte slice not to share leveldb underlaying slice array
	keyItem, err := f.decodeKeyFunc(append([]byte(nil), key...))
	if err != nil {
		return i, fmt.Errorf("decode key: %w", err)
	}
	// create a copy of value byte slice not to share leveldb underlaying slice array
	valueItem, err := f.decodeValueFunc(keyItem, append([]byte(nil), it.Value()...))
	if err != nil {
		return i, fmt.Errorf("decode value: %w", err)
	}
	return keyItem.Merge(valueItem), it.Error()
}

// Last returns the last item in the Index which encoded key starts with a prefix.
// If the prefix is nil, the last element of the whole index is returned.
// If Index has no elements, a leveldb.ErrNotFound error is returned.
func (f Index) Last(prefix []byte) (i Item, err error) {
	it := f.db.NewIterator()
	defer it.Release()

	// get the next prefix in line
	// since leveldb iterator Seek seeks to the
	// next key if the key that it seeks to is not found
	// and by getting the previous key, the last one for the
	// actual prefix is found
	nextPrefix := incByteSlice(prefix)
	l := len(prefix)

	if l > 0 && nextPrefix != nil {
		it.Seek(append(f.prefix, nextPrefix...))
		it.Prev()
	} else {
		it.Last()
	}

	totalPrefix := append(f.prefix, prefix...)
	return f.itemFromIterator(it, totalPrefix)
}

// incByteSlice returns the byte slice of the same size
// of the provided one that is by one incremented in its
// total value. If all bytes in provided slice are equal
// to 255 a nil slice would be returned indicating that
// increment can not happen for the same length.
func incByteSlice(b []byte) (next []byte) {
	l := len(b)
	next = make([]byte, l)
	copy(next, b)
	for i := l - 1; i >= 0; i-- {
		if b[i] == 255 {
			next[i] = 0
		} else {
			next[i] = b[i] + 1
			return next
		}
	}
	return nil
}

// Count returns the number of items in index.
func (f Index) Count() (count int, err error) {
	it := f.db.NewIterator()
	defer it.Release()

	for ok := it.Seek(f.prefix); ok; ok = it.Next() {
		key := it.Key()
		if key[0] != f.prefix[0] {
			break
		}
		count++
	}
	return count, it.Error()
}

// CountFrom returns the number of items in index keys
// starting from the key encoded from the provided Item.
func (f Index) CountFrom(start Item) (count int, err error) {
	startKey, err := f.encodeKeyFunc(start)
	if err != nil {
		return 0, fmt.Errorf("encode key: %w", err)
	}
	it := f.db.NewIterator()
	defer it.Release()

	for ok := it.Seek(startKey); ok; ok = it.Next() {
		key := it.Key()
		if key[0] != f.prefix[0] {
			break
		}
		count++
	}
	return count, it.Error()
}
