// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storer/internal"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstore"
	pinstore "github.com/ethersphere/bee/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/traversal"
	"golang.org/x/sync/errgroup"
)

// epochKey implements storage.Item and is used to store the epoch in the
// store. It is used to check if the epoch migration has already been
// performed.
type epochKey struct{}

func (epochKey) Namespace() string { return "localstore" }

func (epochKey) ID() string { return "epoch" }

// this is a key-only item, so we don't need to marshal/unmarshal
func (epochKey) Marshal() ([]byte, error) { return nil, nil }

func (epochKey) Unmarshal([]byte) error { return nil }

func (epochKey) Clone() storage.Item { return epochKey{} }

func (epochKey) String() string { return "localstore-epoch" }

var (
	_ internal.Storage  = (*putOpStorage)(nil)
	_ chunkstore.Sharky = (*putOpStorage)(nil)
)

// putOpStorage implements the internal.Storage interface which is used by
// the internal component stores to store chunks. It also implements the sharky interface
// which uses the recovery mechanism to recover chunks without moving them.
type putOpStorage struct {
	chunkstore.Sharky

	store    storage.BatchedStore
	location sharky.Location
	recovery sharkyRecover
}

func (p *putOpStorage) IndexStore() storage.BatchedStore { return p.store }

func (p *putOpStorage) ChunkStore() storage.ChunkStore {
	return chunkstore.New(p.store, p)
}

// Write implements the sharky.Store interface. It uses the sharky recovery mechanism
// to recover chunks without moving them. The location returned is the same as the
// one passed in. This is present in the old localstore indexes.
func (p *putOpStorage) Write(_ context.Context, _ []byte) (sharky.Location, error) {
	return p.location, p.recovery.Add(p.location)
}

type reservePutter interface {
	Put(context.Context, internal.Storage, swarm.Chunk) (bool, error)
	AddSize(int)
	Size() int
}

type sharkyRecover interface {
	Add(sharky.Location) error
	Read(context.Context, sharky.Location, []byte) error
}

// epochMigration performs the initial migration if it hasnt been done already. It
// reads the old indexes and writes them in the new format.  It only migrates the
// reserve and pinned chunks. It also creates the new epoch key in the store to
// indicate that the migration has been performed. Due to a bug in the old localstore
// pinned chunks are not always present in the pinned index. So we do a best-effort
// migration of the pinning index. If the migration fails, the user can re-pin
// the chunks using the stewardship endpoint if the stamps used to upload them are
// still valid.
func epochMigration(
	ctx context.Context,
	path string,
	stateStore storage.StateStorer,
	store storage.BatchedStore,
	reserve reservePutter,
	recovery sharkyRecover,
	logger log.Logger,
) error {
	has, err := store.Has(epochKey{})
	if err != nil {
		return fmt.Errorf("has epoch key: %w", err)
	}

	if has {
		return nil
	}

	logger.Debug("started", "path", path, "start_time", time.Now())

	dbshed, err := shed.NewDB(path, nil)
	if err != nil {
		return fmt.Errorf("shed.NewDB: %w", err)
	}

	defer func() {
		if dbshed != nil {
			dbshed.Close()
		}
	}()

	pullIndex, retrievalDataIndex, err := initShedIndexes(dbshed, swarm.ZeroAddress)
	if err != nil {
		return fmt.Errorf("initShedIndexes: %w", err)
	}

	chunkCount, err := retrievalDataIndex.Count()
	if err != nil {
		return fmt.Errorf("retrievalDataIndex count: %w", err)
	}

	pullIdxCnt, _ := pullIndex.Count()

	logger.Debug("index counts", "retrieval index", chunkCount, "pull index", pullIdxCnt)

	e := &epochMigrator{
		stateStore:         stateStore,
		store:              store,
		recovery:           recovery,
		reserve:            reserve,
		pullIndex:          pullIndex,
		retrievalDataIndex: retrievalDataIndex,
		logger:             logger,
	}

	if e.reserve != nil && chunkCount > 0 {
		err = e.migrateReserve(ctx)
		if err != nil {
			return err
		}
	}

	if e.stateStore != nil && chunkCount > 0 {
		err = e.migratePinning(ctx)
		if err != nil {
			return err
		}
	}

	dbshed.Close()
	dbshed = nil

	matches, err := filepath.Glob(filepath.Join(path, "*"))
	if err != nil {
		return err
	}

	for _, m := range matches {
		if !strings.Contains(m, indexPath) && !strings.Contains(m, sharkyPath) {
			err = os.Remove(m)
			if err != nil {
				return err
			}
		}
	}

	return store.Put(epochKey{})
}

func initShedIndexes(dbshed *shed.DB, baseAddress swarm.Address) (pullIndex shed.Index, retrievalDataIndex shed.Index, err error) {
	// pull index allows history and live syncing per po bin
	pullIndex, err = dbshed.NewIndex("PO|BinID->Hash", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			key = make([]byte, 9)
			key[0] = swarm.Proximity(baseAddress.Bytes(), fields.Address)
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
		return shed.Index{}, shed.Index{}, err
	}

	// Index storing actual chunk address, data and bin id.
	headerSize := 16 + postage.StampSize
	retrievalDataIndex, err = dbshed.NewIndex("Address->StoreTimestamp|BinID|BatchID|BatchIndex|Sig|Location", shed.IndexFuncs{
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
		return shed.Index{}, shed.Index{}, err
	}

	return pullIndex, retrievalDataIndex, nil
}

// epochMigrator is a helper struct for migrating epoch data. It is used to house
// the main logic of the migration so that it can be tested. Also it houses the
// dependencies of the migration logic.
type epochMigrator struct {
	stateStore         storage.StateStorer
	store              storage.BatchedStore
	recovery           sharkyRecover
	reserve            reservePutter
	pullIndex          shed.Index
	retrievalDataIndex shed.Index
	logger             log.Logger
}

func (e *epochMigrator) migrateReserve(ctx context.Context) error {
	type putOp struct {
		pIdx  shed.Item
		chunk swarm.Chunk
		loc   sharky.Location
	}

	e.logger.Debug("migrating reserve contents")

	opChan := make(chan putOp, 4)
	eg, egCtx := errgroup.WithContext(ctx)

	for i := 0; i < 4; i++ {
		eg.Go(func() error {
			for {
				select {
				case <-egCtx.Done():
					return egCtx.Err()
				case op, more := <-opChan:
					if !more {
						return nil
					}
					pStorage := &putOpStorage{
						store:    e.store,
						location: op.loc,
						recovery: e.recovery,
					}

					switch newIdx, err := e.reserve.Put(egCtx, pStorage, op.chunk); {
					case err != nil:
						return err
					case newIdx:
						e.reserve.AddSize(1)
					}
				}
			}
		})
	}

	err := func() error {
		defer close(opChan)

		return e.pullIndex.Iterate(func(i shed.Item) (stop bool, err error) {
			addr := swarm.NewAddress(i.Address)

			item, err := e.retrievalDataIndex.Get(i)
			if err != nil {
				e.logger.Debug("retrieval data index read failed", "chunk_address", addr, "error", err)
				return false, nil //continue
			}

			l, err := sharky.LocationFromBinary(item.Location)
			if err != nil {
				e.logger.Debug("location from binary failed", "chunk_address", addr, "error", err)
				return false, err
			}

			chData := make([]byte, l.Length)
			err = e.recovery.Read(ctx, l, chData)
			if err != nil {
				e.logger.Debug("reading location failed", "chunk_address", addr, "error", err)
				return false, nil // continue
			}

			ch := swarm.NewChunk(addr, chData).
				WithStamp(postage.NewStamp(item.BatchID, item.Index, item.Timestamp, item.Sig))

			select {
			case <-egCtx.Done():
				return true, egCtx.Err()
			case opChan <- putOp{pIdx: i, chunk: ch, loc: l}:
			}
			return false, nil
		}, nil)
	}()
	if err != nil {
		return err
	}

	if err := eg.Wait(); err != nil {
		return err
	}

	e.logger.Debug("migrating reserve contents done", "reserve_size", e.reserve.Size())

	return nil
}

const pinStorePrefix = "root-pin"

func (e *epochMigrator) migratePinning(ctx context.Context) error {
	pinChan := make(chan swarm.Address, 4)
	eg, egCtx := errgroup.WithContext(ctx)

	pStorage := &putOpStorage{
		store:    e.store,
		recovery: e.recovery,
	}
	var mu sync.Mutex // used to protect pStorage.location

	traverser := traversal.New(
		storage.GetterFunc(func(ctx context.Context, addr swarm.Address) (ch swarm.Chunk, err error) {
			i := shed.Item{
				Address: addr.Bytes(),
			}
			item, err := e.retrievalDataIndex.Get(i)
			if err != nil {
				return nil, err
			}

			l, err := sharky.LocationFromBinary(item.Location)
			if err != nil {
				return nil, err
			}

			chData := make([]byte, l.Length)
			err = e.recovery.Read(ctx, l, chData)
			if err != nil {
				return nil, err
			}

			return swarm.NewChunk(addr, chData), nil
		}),
		pStorage.ChunkStore(),
	)

	e.logger.Debug("migrating pinning collections, if all the chunks in the collection" +
		" are not found locally, the collection will not be migrated. Users will have to" +
		" re-pin the content using the stewardship API. The migration will print out the failed" +
		" collections at the end.")

	for i := 0; i < 4; i++ {
		eg.Go(func() error {
			for {
				select {
				case <-egCtx.Done():
					return egCtx.Err()
				case addr, more := <-pinChan:
					if !more {
						return nil
					}

					pinningPutter, err := pinstore.NewCollection(pStorage)
					if err != nil {
						return err
					}

					traverserFn := func(chAddr swarm.Address) error {
						item, err := e.retrievalDataIndex.Get(shed.Item{Address: chAddr.Bytes()})
						if err != nil {
							return err
						}

						l, err := sharky.LocationFromBinary(item.Location)
						if err != nil {
							return err
						}
						ch := swarm.NewChunk(chAddr, nil)

						mu.Lock()
						pStorage.location = l
						err = pinningPutter.Put(egCtx, pStorage, pStorage.IndexStore(), ch)
						if err != nil {
							mu.Unlock()
							return err
						}
						mu.Unlock()

						return nil
					}

					err = func() error {
						if err := traverser.Traverse(egCtx, addr, traverserFn); err != nil {
							return err
						}

						if err := pinningPutter.Close(pStorage, pStorage.IndexStore(), addr); err != nil {
							return err
						}
						return nil
					}()

					_ = e.stateStore.Delete(fmt.Sprintf("%s-%s", pinStorePrefix, addr))

					// do not fail the entire migration if the collection is not migrated
					if err != nil {
						e.logger.Debug("pinning collection migration failed", "collection_root_address", addr, "error", err)
					} else {
						e.logger.Debug("pinning collection migration successful", "collection_root_address", addr)
					}
				}
			}
		})
	}

	err := func() error {
		defer close(pinChan)

		return e.stateStore.Iterate(pinStorePrefix, func(key, value []byte) (stop bool, err error) {
			var ref swarm.Address
			if err := json.Unmarshal(value, &ref); err != nil {
				return true, fmt.Errorf("pinning: unmarshal pin reference: %w", err)
			}
			select {
			case <-egCtx.Done():
				return true, egCtx.Err()
			case pinChan <- ref:
			}
			return false, nil
		})
	}()
	if err != nil {
		return err
	}

	if err := eg.Wait(); err != nil {
		return err
	}

	e.logger.Debug("migrating pinning collections done")

	return nil
}
