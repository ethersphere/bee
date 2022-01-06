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

package localstore

import (
	"context"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pinning"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore"
	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"github.com/syndtr/goleveldb/leveldb"
)

var _ storage.Storer = &DB{}

var (
	// ErrInvalidMode is retuned when an unknown Mode
	// is provided to the function.
	ErrInvalidMode = errors.New("invalid mode")
)

var (
	// Default value for Capacity DB option.
	defaultCacheCapacity uint64 = 1000000
	// Limit the number of goroutines created by Getters
	// that call updateGC function. Value 0 sets no limit.
	maxParallelUpdateGC = 1000

	// values needed to adjust subscription trigger
	// buffer time.
	flipFlopBufferDuration    = 150 * time.Millisecond
	flipFlopWorstCaseDuration = 10 * time.Second
)

const (
	// 32 * 312500 chunks = 1000000 chunks (40GB)
	// currently this size is enforced by the localstore
	sharkyNoOfShards    int    = 32
	sharkyPerShardLimit uint32 = 312500
	sharkyDirtyFileName string = ".DIRTY"
)

// DB is the local store implementation and holds
// database related objects.
type DB struct {
	shed *shed.DB
	// sharky instance
	sharky *sharky.Store

	fdirty *os.File // LOCK file handle
	tags   *tags.Tags

	// stateStore is needed to access the pinning Service.Pins() method.
	stateStore storage.StateStorer

	// schema name of loaded data
	schemaName shed.StringField

	// retrieval indexes
	retrievalDataIndex   shed.Index
	retrievalAccessIndex shed.Index
	// push syncing index
	pushIndex shed.Index
	// push syncing subscriptions triggers
	pushTriggers   []chan<- struct{}
	pushTriggersMu sync.RWMutex

	// pull syncing index
	pullIndex shed.Index
	// pull syncing subscriptions triggers per bin
	pullTriggers   map[uint8][]chan<- struct{}
	pullTriggersMu sync.RWMutex

	// binIDs stores the latest chunk serial ID for every
	// proximity order bin
	binIDs shed.Uint64Vector

	// garbage collection index
	gcIndex shed.Index

	// pin files Index
	pinIndex shed.Index

	// postage chunks index
	postageChunksIndex shed.Index

	// postage radius index
	postageRadiusIndex shed.Index

	// postage index index
	postageIndexIndex shed.Index

	// field that stores number of intems in gc index
	gcSize shed.Uint64Field

	// field that stores the size of the reserve
	reserveSize shed.Uint64Field

	// garbage collection is triggered when gcSize exceeds
	// the cacheCapacity value
	cacheCapacity uint64

	// the size of the reserve in chunks
	reserveCapacity uint64

	unreserveFunc func(postage.UnreserveIteratorFn) error

	// triggers garbage collection event loop
	collectGarbageTrigger chan struct{}

	// triggers reserve eviction event loop
	reserveEvictionTrigger chan struct{}

	// a buffered channel acting as a semaphore
	// to limit the maximal number of goroutines
	// created by Getters to call updateGC function
	updateGCSem chan struct{}
	// a wait group to ensure all updateGC goroutines
	// are done before closing the database
	updateGCWG sync.WaitGroup

	// baseKey is the overlay address
	baseKey []byte

	batchMu sync.Mutex

	// gcRunning is true while GC is running. it is
	// used to avoid touching dirty gc index entries
	// while garbage collecting.
	gcRunning bool

	// dirtyAddresses are marked while gc is running
	// in order to avoid the removal of dirty entries.
	dirtyAddresses []swarm.Address

	// this channel is closed when close function is called
	// to terminate other goroutines
	close chan struct{}

	// context
	ctx context.Context
	// the cancelation function from the context
	cancel context.CancelFunc

	// protect Close method from exiting before
	// garbage collection and gc size write workers
	// are done
	collectGarbageWorkerDone  chan struct{}
	reserveEvictionWorkerDone chan struct{}

	// wait for all subscriptions to finish before closing
	// underlaying leveldb to prevent possible panics from
	// iterators
	subscriptionsWG sync.WaitGroup

	metrics metrics

	logger logging.Logger
}

// Options struct holds optional parameters for configuring DB.
type Options struct {
	// Capacity is a limit that triggers garbage collection when
	// number of items in gcIndex equals or exceeds it.
	Capacity uint64
	// ReserveCapacity is the capacity of the reserve.
	ReserveCapacity uint64
	// UnreserveFunc is an iterator needed to facilitate reserve
	// eviction once ReserveCapacity is reached.
	UnreserveFunc func(postage.UnreserveIteratorFn) error
	// OpenFilesLimit defines the upper bound of open files that the
	// the localstore should maintain at any point of time. It is
	// passed on to the shed constructor.
	OpenFilesLimit uint64
	// BlockCacheCapacity defines the block cache capacity and is passed
	// on to shed.
	BlockCacheCapacity uint64
	// WriteBuffer defines the size of writer buffer and is passed on to shed.
	WriteBufferSize uint64
	// DisableSeeksCompaction toggles the seek driven compactions feature on leveldb
	// and is passed on to shed.
	DisableSeeksCompaction bool

	// MetricsPrefix defines a prefix for metrics names.
	MetricsPrefix string
	Tags          *tags.Tags
}

type memFS struct {
	afero.Fs
}

func (m *memFS) Open(path string) (fs.File, error) {
	return m.Fs.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
}

type dirFS struct {
	basedir string
}

func (d *dirFS) Open(path string) (fs.File, error) {
	return os.OpenFile(filepath.Join(d.basedir, path), os.O_RDWR|os.O_CREATE, 0644)
}

// New returns a new DB.  All fields and indexes are initialized
// and possible conflicts with schema from existing database is checked.
// One goroutine for writing batches is created.
func New(path string, baseKey []byte, ss storage.StateStorer, o *Options, logger logging.Logger) (db *DB, err error) {
	if o == nil {
		// default options
		o = &Options{
			Capacity:        defaultCacheCapacity,
			ReserveCapacity: uint64(batchstore.Capacity),
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	db = &DB{
		stateStore:      ss,
		cacheCapacity:   o.Capacity,
		reserveCapacity: o.ReserveCapacity,
		unreserveFunc:   o.UnreserveFunc,
		baseKey:         baseKey,
		tags:            o.Tags,
		ctx:             ctx,
		cancel:          cancel,
		// channel collectGarbageTrigger
		// needs to be buffered with the size of 1
		// to signal another event if it
		// is triggered during already running function
		collectGarbageTrigger:     make(chan struct{}, 1),
		reserveEvictionTrigger:    make(chan struct{}, 1),
		close:                     make(chan struct{}),
		collectGarbageWorkerDone:  make(chan struct{}),
		reserveEvictionWorkerDone: make(chan struct{}),
		metrics:                   newMetrics(),
		logger:                    logger,
	}
	if db.cacheCapacity == 0 {
		db.cacheCapacity = defaultCacheCapacity
	}

	capacityMB := float64((db.cacheCapacity+uint64(batchstore.Capacity))*swarm.ChunkSize) * 9.5367431640625e-7

	if capacityMB <= 1000 {
		db.logger.Infof("database capacity: %d chunks (approximately %fMB)", db.cacheCapacity, capacityMB)
	} else {
		db.logger.Infof("database capacity: %d chunks (approximately %0.1fGB)", db.cacheCapacity, capacityMB/1000)
	}

	if maxParallelUpdateGC > 0 {
		db.updateGCSem = make(chan struct{}, maxParallelUpdateGC)
	}

	shedOpts := &shed.Options{
		OpenFilesLimit:         o.OpenFilesLimit,
		BlockCacheCapacity:     o.BlockCacheCapacity,
		WriteBufferSize:        o.WriteBufferSize,
		DisableSeeksCompaction: o.DisableSeeksCompaction,
	}

	if withinRadiusFn == nil {
		withinRadiusFn = withinRadius
	}

	db.shed, err = shed.NewDB(path, shedOpts)
	if err != nil {
		return nil, err
	}

	// Identify current storage schema by arbitrary name.
	db.schemaName, err = db.shed.NewStringField("schema-name")
	if err != nil {
		return nil, err
	}
	schemaName, err := db.schemaName.Get()
	if err != nil && !errors.Is(err, leveldb.ErrNotFound) {
		return nil, err
	}
	if schemaName == "" {
		// initial new localstore run
		err := db.schemaName.Put(DBSchemaCurrent)
		if err != nil {
			return nil, err
		}
	} else {
		// execute possible migrations
		err = db.migrate(schemaName)
		if err != nil {
			return nil, err
		}
	}

	// Persist gc size.
	db.gcSize, err = db.shed.NewUint64Field("gc-size")
	if err != nil {
		return nil, err
	}

	// reserve size
	db.reserveSize, err = db.shed.NewUint64Field("reserve-size")
	if err != nil {
		return nil, err
	}

	// instantiate sharky instance
	var sharkyBase fs.FS
	if path == "" {
		sharkyBase = &memFS{Fs: afero.NewMemMapFs()}
	} else {
		sharkyBasePath := filepath.Join(path, "sharky")
		if _, err := os.Stat(sharkyBasePath); os.IsNotExist(err) {
			err := os.Mkdir(sharkyBasePath, 0775)
			if err != nil {
				return nil, err
			}
		}
		sharkyBase = &dirFS{basedir: sharkyBasePath}

		// check whether file already existed, and if so, initiate recovery process
		if isDirtyShutdown(path) {
			//recovery
			locOrErr, err := recovery(db)
			if err != nil {
				return nil, err
			}

			recoverySharky, err := sharky.NewRecovery(sharkyBasePath, sharkyNoOfShards, sharkyPerShardLimit, swarm.ChunkWithSpanSize)
			if err != nil {
				return nil, err
			}

			for l := range locOrErr {
				if l.err != nil {
					db.logger.Warning("error reading sharky location", l.err)
					continue
				}

				err = recoverySharky.Add(l.loc)
				if err != nil {
					return nil, err
				}
			}

			err = recoverySharky.Save()
			if err != nil {
				return nil, err
			}

			err = recoverySharky.Close()
			if err != nil {
				return nil, err
			}

			// remove the existing dirty file so a new one can be initialized for
			// the new instance
			err = os.Remove(filepath.Join(path, sharkyDirtyFileName))
			if err != nil {
				return nil, err
			}
		}

		fdirty, err := createDirtyFile(path)
		if err != nil {
			return nil, err
		}
		db.fdirty = fdirty
	}

	db.sharky, err = sharky.New(sharkyBase, sharkyNoOfShards, sharkyPerShardLimit, swarm.ChunkWithSpanSize)
	if err != nil {
		return nil, err
	}

	// Index storing actual chunk address, data and bin id.
	headerSize := 16 + postage.StampSize
	db.retrievalDataIndex, err = db.shed.NewIndex("Address->StoreTimestamp|BinID|BatchID|BatchIndex|Sig|Location", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			return fields.Address, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.Address = key
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			b := make([]byte, headerSize)
			binary.BigEndian.PutUint64(b[:8], fields.BinID)
			binary.BigEndian.PutUint64(b[8:16], uint64(fields.StoreTimestamp))
			stamp, err := postage.NewStamp(fields.BatchID, fields.Index, fields.Timestamp, fields.Sig).MarshalBinary()
			if err != nil {
				return nil, err
			}
			copy(b[16:], stamp)
			value = append(b, fields.Location...)
			return value, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.StoreTimestamp = int64(binary.BigEndian.Uint64(value[8:16]))
			e.BinID = binary.BigEndian.Uint64(value[:8])
			stamp := new(postage.Stamp)
			if err = stamp.UnmarshalBinary(value[16:headerSize]); err != nil {
				return e, err
			}
			e.BatchID = stamp.BatchID()
			e.Index = stamp.Index()
			e.Timestamp = stamp.Timestamp()
			e.Sig = stamp.Sig()
			e.Location = value[headerSize:]
			return e, nil
		},
	})
	if err != nil {
		return nil, err
	}
	// Index storing access timestamp for a particular address.
	// It is needed in order to update gc index keys for iteration order.
	db.retrievalAccessIndex, err = db.shed.NewIndex("Address->AccessTimestamp", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			return fields.Address, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.Address = key
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			b := make([]byte, 8)
			binary.BigEndian.PutUint64(b, uint64(fields.AccessTimestamp))
			return b, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.AccessTimestamp = int64(binary.BigEndian.Uint64(value))
			return e, nil
		},
	})
	if err != nil {
		return nil, err
	}
	// pull index allows history and live syncing per po bin
	db.pullIndex, err = db.shed.NewIndex("PO|BinID->Hash", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			key = make([]byte, 9)
			key[0] = db.po(swarm.NewAddress(fields.Address))
			binary.BigEndian.PutUint64(key[1:9], fields.BinID)
			return key, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.BinID = binary.BigEndian.Uint64(key[1:9])
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			value = make([]byte, 64) // 32 bytes address, 32 bytes batch id
			copy(value, fields.Address)
			copy(value[32:], fields.BatchID)
			return value, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.Address = value[:32]
			e.BatchID = value[32:64]
			return e, nil
		},
	})
	if err != nil {
		return nil, err
	}
	// create a vector for bin IDs
	db.binIDs, err = db.shed.NewUint64Vector("bin-ids")
	if err != nil {
		return nil, err
	}
	// create a pull syncing triggers used by SubscribePull function
	db.pullTriggers = make(map[uint8][]chan<- struct{})
	// push index contains as yet unsynced chunks
	db.pushIndex, err = db.shed.NewIndex("StoreTimestamp|Hash->Tags", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			key = make([]byte, 40)
			binary.BigEndian.PutUint64(key[:8], uint64(fields.StoreTimestamp))
			copy(key[8:], fields.Address)
			return key, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.Address = key[8:]
			e.StoreTimestamp = int64(binary.BigEndian.Uint64(key[:8]))
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			tag := make([]byte, 4)
			binary.BigEndian.PutUint32(tag, fields.Tag)
			return tag, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			if len(value) == 4 { // only values with tag should be decoded
				e.Tag = binary.BigEndian.Uint32(value)
			}
			return e, nil
		},
	})
	if err != nil {
		return nil, err
	}
	// create a push syncing triggers used by SubscribePush function
	db.pushTriggers = make([]chan<- struct{}, 0)
	// gc index for removable chunk ordered by ascending last access time
	db.gcIndex, err = db.shed.NewIndex("AccessTimestamp|BinID|Hash->BatchID|BatchIndex", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			b := make([]byte, 16, 16+len(fields.Address))
			binary.BigEndian.PutUint64(b[:8], uint64(fields.AccessTimestamp))
			binary.BigEndian.PutUint64(b[8:16], fields.BinID)
			key = append(b, fields.Address...)
			return key, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.AccessTimestamp = int64(binary.BigEndian.Uint64(key[:8]))
			e.BinID = binary.BigEndian.Uint64(key[8:16])
			e.Address = key[16:]
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			value = make([]byte, 40)
			copy(value, fields.BatchID)
			copy(value[32:], fields.Index)
			return value, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.BatchID = make([]byte, 32)
			copy(e.BatchID, value[:32])
			e.Index = make([]byte, postage.IndexSize)
			copy(e.Index, value[32:])
			return e, nil
		},
	})
	if err != nil {
		return nil, err
	}

	// Create a index structure for storing pinned chunks and their pin counts
	db.pinIndex, err = db.shed.NewIndex("Hash->PinCounter", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			return fields.Address, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.Address = key
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			b := make([]byte, 8)
			binary.BigEndian.PutUint64(b[:8], fields.PinCounter)
			return b, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.PinCounter = binary.BigEndian.Uint64(value[:8])
			return e, nil
		},
	})
	if err != nil {
		return nil, err
	}

	db.postageChunksIndex, err = db.shed.NewIndex("BatchID|PO|Hash->nil", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			key = make([]byte, 65)
			copy(key[:32], fields.BatchID)
			key[32] = db.po(swarm.NewAddress(fields.Address))
			copy(key[33:], fields.Address)
			return key, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.BatchID = key[:32]
			e.Address = key[33:65]
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			return nil, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			return e, nil
		},
	})
	if err != nil {
		return nil, err
	}

	db.postageRadiusIndex, err = db.shed.NewIndex("BatchID->Radius", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			key = make([]byte, 32)
			copy(key[:32], fields.BatchID)
			return key, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.BatchID = key[:32]
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			return []byte{fields.Radius}, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.Radius = value[0]
			return e, nil
		},
	})
	if err != nil {
		return nil, err
	}

	db.postageIndexIndex, err = db.shed.NewIndex("BatchID|BatchIndex->Hash|Timestamp", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			key = make([]byte, 40)
			copy(key[:32], fields.BatchID)
			copy(key[32:40], fields.Index)
			return key, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.BatchID = key[:32]
			e.Index = key[32:40]
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			value = make([]byte, 40)
			copy(value, fields.Address)
			copy(value[32:], fields.Timestamp)
			return value, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.Address = value[:32]
			e.Timestamp = value[32:]
			return e, nil
		},
	})
	if err != nil {
		return nil, err
	}

	// start garbage collection worker
	go db.collectGarbageWorker()
	go db.reserveEvictionWorker()
	return db, nil
}

// Close closes the underlying database.
func (db *DB) Close() (err error) {
	close(db.close)
	db.cancel()

	// wait for all handlers to finish
	done := make(chan struct{})
	go func() {
		db.updateGCWG.Wait()
		// wait for gc worker to
		// return before closing the shed
		<-db.collectGarbageWorkerDone
		<-db.reserveEvictionWorkerDone
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		db.logger.Errorf("localstore closed with still active goroutines")
		// Print a full goroutine dump to debug blocking.
		// TODO: use a logger to write a goroutine profile
		prof := pprof.Lookup("goroutine")
		err = prof.WriteTo(os.Stdout, 2)
		if err != nil {
			return err
		}
	}
	// todo instrument these
	if err = db.sharky.Close(); err != nil {
		return err
	}
	if err = db.shed.Close(); err != nil {
		return err
	}

	if db.fdirty != nil {
		if err = db.fdirty.Close(); err != nil {
			return err
		}
		if err = os.Remove(db.fdirty.Name()); err != nil {
			return err
		}
	}

	return nil
}

// po computes the proximity order between the address
// and database base key.
func (db *DB) po(addr swarm.Address) (bin uint8) {
	return swarm.Proximity(db.baseKey, addr.Bytes())
}

// DebugIndices returns the index sizes for all indexes in localstore
// the returned map keys are the index name, values are the number of elements in the index
func (db *DB) DebugIndices() (indexInfo map[string]int, err error) {
	indexInfo = make(map[string]int)
	for k, v := range map[string]shed.Index{
		"retrievalDataIndex":   db.retrievalDataIndex,
		"retrievalAccessIndex": db.retrievalAccessIndex,
		"pushIndex":            db.pushIndex,
		"pullIndex":            db.pullIndex,
		"gcIndex":              db.gcIndex,
		"pinIndex":             db.pinIndex,
		"postageChunksIndex":   db.postageChunksIndex,
		"postageRadiusIndex":   db.postageRadiusIndex,
	} {
		indexSize, err := v.Count()
		if err != nil {
			return indexInfo, err
		}
		indexInfo[k] = indexSize
	}
	val, err := db.gcSize.Get()
	if err != nil {
		return indexInfo, err
	}
	indexInfo["gcSize"] = int(val)

	return indexInfo, err
}

// stateStoreHasPins returns true if the state-store
// contains any pins, otherwise false is returned.
func (db *DB) stateStoreHasPins() (bool, error) {
	pins, err := pinning.NewService(nil, db.stateStore, nil).Pins()
	if err != nil {
		return false, err
	}
	return len(pins) > 0, nil
}

// chunkToItem creates new Item with data provided by the Chunk.
func chunkToItem(ch swarm.Chunk) shed.Item {
	return shed.Item{
		Address:     ch.Address().Bytes(),
		Data:        ch.Data(),
		Tag:         ch.TagID(),
		BatchID:     ch.Stamp().BatchID(),
		Index:       ch.Stamp().Index(),
		Timestamp:   ch.Stamp().Timestamp(),
		Sig:         ch.Stamp().Sig(),
		Depth:       ch.Depth(),
		Radius:      ch.Radius(),
		BucketDepth: ch.BucketDepth(),
		Immutable:   ch.Immutable(),
	}
}

// addressToItem creates new Item with a provided address.
func addressToItem(addr swarm.Address) shed.Item {
	return shed.Item{
		Address: addr.Bytes(),
	}
}

// addressesToItems constructs a slice of Items with only
// addresses set on them.
func addressesToItems(addrs ...swarm.Address) []shed.Item {
	items := make([]shed.Item, len(addrs))
	for i, addr := range addrs {
		items[i] = shed.Item{
			Address: addr.Bytes(),
		}
	}
	return items
}

// now is a helper function that returns a current unix timestamp
// in UTC timezone.
// It is set in the init function for usage in production, and
// optionally overridden in tests for data validation.
var now func() int64

func init() {
	// set the now function
	now = func() (t int64) {
		return time.Now().UTC().UnixNano()
	}
}

// totalTimeMetric logs a message about time between provided start time
// and the time when the function is called and sends a resetting timer metric
// with provided name appended with ".total-time".
func totalTimeMetric(metric prometheus.Counter, start time.Time) {
	totalTime := time.Since(start)
	metric.Add(float64(totalTime))
}

func isDirtyShutdown(path string) bool {
	_, err := os.Stat(filepath.Join(path, sharkyDirtyFileName))
	isClean := errors.Is(err, fs.ErrNotExist) // missing lock file implies a clean exit

	return !isClean
}

func createDirtyFile(path string) (f *os.File, err error) {
	path = filepath.Join(path, sharkyDirtyFileName)
	f, err = os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0644)

	return
}
