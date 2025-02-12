// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package upload

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"time"

	"github.com/ethersphere/bee/v2/pkg/encryption"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/storageutil"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

// now returns the current time.Time; used in testing.
var now = time.Now

var (
	// errPushItemMarshalAddressIsZero is returned when trying
	// to marshal a pushItem with an address that is zero.
	errPushItemMarshalAddressIsZero = errors.New("marshal pushItem: address is zero")
	// errPushItemMarshalBatchInvalid is returned when trying to
	// marshal a pushItem with invalid batch
	errPushItemMarshalBatchInvalid = errors.New("marshal pushItem: batch is invalid")
	// errPushItemUnmarshalInvalidSize is returned when trying
	// to unmarshal buffer that is not of size pushItemSize.
	errPushItemUnmarshalInvalidSize = errors.New("unmarshal pushItem: invalid size")
)

// pushItemSize is the size of a marshaled pushItem.
const pushItemSize = 8 + 2*swarm.HashSize + 8

const uploadScope = "upload"

var _ storage.Item = (*pushItem)(nil)

// pushItem is an store.Item that represents data relevant to push.
// The key is a combination of Timestamp, Address and postage stamp, where the
// Timestamp provides an order to iterate.
type pushItem struct {
	Timestamp int64
	Address   swarm.Address
	BatchID   []byte
	TagID     uint64
}

// ID implements the storage.Item interface.
func (i pushItem) ID() string {
	return fmt.Sprintf("%d/%s/%s", i.Timestamp, i.Address.ByteString(), string(i.BatchID))
}

// Namespace implements the storage.Item interface.
func (i pushItem) Namespace() string {
	return "pushIndex"
}

// Marshal implements the storage.Item interface.
// If the Address is zero, an error is returned.
func (i pushItem) Marshal() ([]byte, error) {
	if i.Address.IsZero() {
		return nil, errPushItemMarshalAddressIsZero
	}
	if len(i.BatchID) != swarm.HashSize {
		return nil, errPushItemMarshalBatchInvalid
	}
	buf := make([]byte, pushItemSize)
	binary.LittleEndian.PutUint64(buf, uint64(i.Timestamp))
	copy(buf[8:], i.Address.Bytes())
	copy(buf[8+swarm.HashSize:8+2*swarm.HashSize], i.BatchID)
	binary.LittleEndian.PutUint64(buf[8+2*swarm.HashSize:], i.TagID)
	return buf, nil
}

// Unmarshal implements the storage.Item interface.
// If the buffer is not of size pushItemSize, an error is returned.
func (i *pushItem) Unmarshal(bytes []byte) error {
	if len(bytes) != pushItemSize {
		return errPushItemUnmarshalInvalidSize
	}
	ni := new(pushItem)
	ni.Timestamp = int64(binary.LittleEndian.Uint64(bytes))
	ni.Address = swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), bytes[8:8+swarm.HashSize]...))
	ni.BatchID = append(make([]byte, 0, swarm.HashSize), bytes[8+swarm.HashSize:8+2*swarm.HashSize]...)
	ni.TagID = binary.LittleEndian.Uint64(bytes[8+2*swarm.HashSize:])
	*i = *ni
	return nil
}

// Clone implements the storage.Item interface.
func (i *pushItem) Clone() storage.Item {
	if i == nil {
		return nil
	}
	return &pushItem{
		Timestamp: i.Timestamp,
		Address:   i.Address.Clone(),
		BatchID:   append([]byte(nil), i.BatchID...),
		TagID:     i.TagID,
	}
}

// String implements the fmt.Stringer interface.
func (i pushItem) String() string {
	return storageutil.JoinFields(i.Namespace(), i.ID())
}

var (
	// errTagIDAddressItemUnmarshalInvalidSize is returned when trying
	// to unmarshal buffer that is not of size tagItemSize.
	errTagItemUnmarshalInvalidSize = errors.New("unmarshal TagItem: invalid size")
)

// tagItemSize is the size of a marshaled TagItem.
const tagItemSize = swarm.HashSize + 7*8

var _ storage.Item = (*TagItem)(nil)

// TagItem is an store.Item that stores information about a session of upload.
type TagItem struct {
	TagID     uint64        // unique identifier for the tag
	Split     uint64        // total no of chunks processed by the splitter for hashing
	Seen      uint64        // total no of chunks already seen
	Stored    uint64        // total no of chunks stored locally on the node
	Sent      uint64        // total no of chunks sent to the neighbourhood
	Synced    uint64        // total no of chunks synced with proof
	Address   swarm.Address // swarm.Address associated with this tag
	StartedAt int64         // start timestamp
}

// ID implements the storage.Item interface.
func (i TagItem) ID() string {
	return strconv.FormatUint(i.TagID, 10)
}

// Namespace implements the storage.Item interface.
func (i TagItem) Namespace() string {
	return "tagItem"
}

// Marshal implements the storage.Item interface.
func (i TagItem) Marshal() ([]byte, error) {
	buf := make([]byte, tagItemSize)
	binary.LittleEndian.PutUint64(buf, i.TagID)
	binary.LittleEndian.PutUint64(buf[8:], i.Split)
	binary.LittleEndian.PutUint64(buf[16:], i.Seen)
	binary.LittleEndian.PutUint64(buf[24:], i.Stored)
	binary.LittleEndian.PutUint64(buf[32:], i.Sent)
	binary.LittleEndian.PutUint64(buf[40:], i.Synced)
	addrBytes := internal.AddressBytesOrZero(i.Address)
	if len(addrBytes) == encryption.ReferenceSize {
		// in case of encrypted reference we use the swarm hash as the address and
		// avoid storing the encryption key
		addrBytes = addrBytes[:swarm.HashSize]
	}
	copy(buf[48:], addrBytes)
	binary.LittleEndian.PutUint64(buf[48+swarm.HashSize:], uint64(i.StartedAt))
	return buf, nil
}

// Unmarshal implements the storage.Item interface.
// If the buffer is not of size tagItemSize, an error is returned.
func (i *TagItem) Unmarshal(bytes []byte) error {
	if len(bytes) != tagItemSize {
		return errTagItemUnmarshalInvalidSize
	}
	ni := new(TagItem)
	ni.TagID = binary.LittleEndian.Uint64(bytes)
	ni.Split = binary.LittleEndian.Uint64(bytes[8:])
	ni.Seen = binary.LittleEndian.Uint64(bytes[16:])
	ni.Stored = binary.LittleEndian.Uint64(bytes[24:])
	ni.Sent = binary.LittleEndian.Uint64(bytes[32:])
	ni.Synced = binary.LittleEndian.Uint64(bytes[40:])
	ni.Address = internal.AddressOrZero(bytes[48 : 48+swarm.HashSize])
	ni.StartedAt = int64(binary.LittleEndian.Uint64(bytes[48+swarm.HashSize:]))
	*i = *ni
	return nil
}

// Clone implements the storage.Item interface.
func (i *TagItem) Clone() storage.Item {
	if i == nil {
		return nil
	}
	return &TagItem{
		TagID:     i.TagID,
		Split:     i.Split,
		Seen:      i.Seen,
		Stored:    i.Stored,
		Sent:      i.Sent,
		Synced:    i.Synced,
		Address:   i.Address.Clone(),
		StartedAt: i.StartedAt,
	}
}

// String implements the fmt.Stringer interface.
func (i TagItem) String() string {
	return storageutil.JoinFields(i.Namespace(), i.ID())
}

var (
	// errUploadItemMarshalAddressIsZero is returned when trying
	// to marshal a uploadItem with an address that is zero.
	errUploadItemMarshalAddressIsZero = errors.New("marshal uploadItem: address is zero")
	// errUploadItemMarshalBatchInvalid is returned when trying to
	// marshal a uploadItem with invalid batch
	errUploadItemMarshalBatchInvalid = errors.New("marshal uploadItem: batch is invalid")
	// errTagIDAddressItemUnmarshalInvalidSize is returned when trying
	// to unmarshal buffer that is not of size uploadItemSize.
	errUploadItemUnmarshalInvalidSize = errors.New("unmarshal uploadItem: invalid size")
)

// uploadItemSize is the size of a marshaled uploadItem.
const uploadItemSize = 3 * 8

var _ storage.Item = (*uploadItem)(nil)

// uploadItem is an store.Item that stores addresses of already seen chunks.
type uploadItem struct {
	Address  swarm.Address
	BatchID  []byte
	TagID    uint64
	Uploaded int64
	Synced   int64

	// IdFunc overrides the ID method.
	// This used to get the ID from the item where the address and batchID were not marshalled.
	IdFunc func() string
}

// ID implements the storage.Item interface.
func (i uploadItem) ID() string {
	if i.IdFunc != nil {
		return i.IdFunc()
	}
	return storageutil.JoinFields(i.Address.ByteString(), string(i.BatchID))
}

// Namespace implements the storage.Item interface.
func (i uploadItem) Namespace() string {
	return "UploadItem"
}

// Marshal implements the storage.Item interface.
// If the Address is zero, an error is returned.
func (i uploadItem) Marshal() ([]byte, error) {
	// Address and BatchID are not part of the marshaled payload. But they are used
	// in they key and hence are required. The Marshaling is done when item is to
	// be stored, so we return errors for these cases.
	if i.Address.IsZero() {
		return nil, errUploadItemMarshalAddressIsZero
	}
	if len(i.BatchID) != swarm.HashSize {
		return nil, errUploadItemMarshalBatchInvalid
	}
	buf := make([]byte, uploadItemSize)
	binary.LittleEndian.PutUint64(buf, i.TagID)
	binary.LittleEndian.PutUint64(buf[8:], uint64(i.Uploaded))
	binary.LittleEndian.PutUint64(buf[16:], uint64(i.Synced))
	return buf, nil
}

// Unmarshal implements the storage.Item interface.
// If the buffer is not of size pushItemSize, an error is returned.
func (i *uploadItem) Unmarshal(bytes []byte) error {
	if len(bytes) != uploadItemSize {
		return errUploadItemUnmarshalInvalidSize
	}
	// The Address and BatchID are required for the key, so it is assumed that
	// they will be filled already. We reuse them during unmarshaling.
	i.TagID = binary.LittleEndian.Uint64(bytes[:8])
	i.Uploaded = int64(binary.LittleEndian.Uint64(bytes[8:16]))
	i.Synced = int64(binary.LittleEndian.Uint64(bytes[16:]))
	return nil
}

// Clone implements the storage.Item interface.
func (i *uploadItem) Clone() storage.Item {
	if i == nil {
		return nil
	}
	return &uploadItem{
		Address:  i.Address.Clone(),
		BatchID:  append([]byte(nil), i.BatchID...),
		TagID:    i.TagID,
		Uploaded: i.Uploaded,
		Synced:   i.Synced,
	}
}

// String implements the fmt.Stringer interface.
func (i uploadItem) String() string {
	return storageutil.JoinFields(i.Namespace(), i.ID())
}

// dirtyTagItemUnmarshalInvalidSize is returned when trying
// to unmarshal buffer that is not of size dirtyTagItemSize.
var errDirtyTagItemUnmarshalInvalidSize = errors.New("unmarshal dirtyTagItem: invalid size")

// dirtyTagItemSize is the size of a marshaled dirtyTagItem.
const dirtyTagItemSize = 8 + 8

type dirtyTagItem struct {
	TagID   uint64
	Started int64
}

// ID implements the storage.Item interface.
func (i dirtyTagItem) ID() string {
	return strconv.FormatUint(i.TagID, 10)
}

// Namespace implements the storage.Item interface.
func (i dirtyTagItem) Namespace() string {
	return "DirtyTagItem"
}

// Marshal implements the storage.Item interface.
func (i dirtyTagItem) Marshal() ([]byte, error) {
	buf := make([]byte, dirtyTagItemSize)
	binary.LittleEndian.PutUint64(buf, i.TagID)
	binary.LittleEndian.PutUint64(buf[8:], uint64(i.Started))
	return buf, nil
}

// Unmarshal implements the storage.Item interface.
func (i *dirtyTagItem) Unmarshal(bytes []byte) error {
	if len(bytes) != dirtyTagItemSize {
		return errDirtyTagItemUnmarshalInvalidSize
	}
	i.TagID = binary.LittleEndian.Uint64(bytes[:8])
	i.Started = int64(binary.LittleEndian.Uint64(bytes[8:]))
	return nil
}

// Clone implements the storage.Item interface.
func (i *dirtyTagItem) Clone() storage.Item {
	if i == nil {
		return nil
	}
	return &dirtyTagItem{
		TagID:   i.TagID,
		Started: i.Started,
	}
}

// String implements the fmt.Stringer interface.
func (i dirtyTagItem) String() string {
	return storageutil.JoinFields(i.Namespace(), i.ID())
}

var (
	// errPutterAlreadyClosed is returned when trying to Put a new chunk
	// after the putter has been closed.
	errPutterAlreadyClosed = errors.New("upload store: putter already closed")

	// errOverwriteOfImmutableBatch is returned when stamp index already
	// exists and the batch is immutable.
	errOverwriteOfImmutableBatch = errors.New("upload store: overwrite of existing immutable batch")

	// errOverwriteOfNewerBatch is returned if a stamp index already exists
	// and the existing chunk with the same stamp index has a newer timestamp.
	errOverwriteOfNewerBatch = errors.New("upload store: overwrite of existing batch with newer timestamp")
)

type uploadPutter struct {
	tagID  uint64
	split  uint64
	seen   uint64
	closed bool
}

// NewPutter returns a new chunk putter associated with the tagID.
// Calls to the Putter must be mutex locked to prevent concurrent upload data races.
func NewPutter(s storage.IndexStore, tagID uint64) (internal.PutterCloserWithReference, error) {
	ti := &TagItem{TagID: tagID}
	has, err := s.Has(ti)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, fmt.Errorf("upload store: tag %d not found: %w", tagID, storage.ErrNotFound)
	}
	err = s.Put(&dirtyTagItem{TagID: tagID, Started: now().UnixNano()})
	if err != nil {
		return nil, err
	}
	return &uploadPutter{
		tagID: ti.TagID,
	}, nil
}

// Put operation will do the following:
// 1.If upload store has already seen this chunk, it will update the tag and return
// 2.For a new chunk it will add:
// - uploadItem entry to keep track of this chunk.
// - pushItem entry to make it available for PushSubscriber
// - add chunk to the chunkstore till it is synced
// The user of the putter MUST mutex lock the call to prevent data-races across multiple upload sessions.
func (u *uploadPutter) Put(ctx context.Context, st transaction.Store, chunk swarm.Chunk) error {
	if u.closed {
		return errPutterAlreadyClosed
	}

	// Check if upload store has already seen this chunk
	ui := &uploadItem{Address: chunk.Address(), BatchID: chunk.Stamp().BatchID()}
	switch exists, err := st.IndexStore().Has(ui); {
	case err != nil:
		return fmt.Errorf("store has item %q call failed: %w", ui, err)
	case exists:
		u.seen++
		u.split++
		return nil
	}

	u.split++

	ui.Uploaded = now().UnixNano()
	ui.TagID = u.tagID

	pi := &pushItem{
		Timestamp: ui.Uploaded,
		Address:   chunk.Address(),
		BatchID:   chunk.Stamp().BatchID(),
		TagID:     u.tagID,
	}

	return errors.Join(
		st.IndexStore().Put(ui),
		st.IndexStore().Put(pi),
		st.ChunkStore().Put(ctx, chunk),
		chunkstamp.Store(st.IndexStore(), uploadScope, chunk),
	)
}

// Close provides the CloseWithReference interface where the session can be associated
// with a swarm reference. This can be useful while keeping track of uploads through
// the tags. It will update the tag. This will be filled with the Split and Seen count
// by the Putter.
func (u *uploadPutter) Close(s storage.IndexStore, addr swarm.Address) error {
	if u.closed {
		return nil
	}

	ti := &TagItem{TagID: u.tagID}
	err := s.Get(ti)
	if err != nil {
		return fmt.Errorf("failed reading tag while closing: %w", err)
	}

	ti.Split += u.split
	ti.Seen += u.seen

	if !addr.IsZero() {
		ti.Address = addr.Clone()
	}

	u.closed = true

	return errors.Join(
		s.Put(ti),
		s.Delete(&dirtyTagItem{TagID: u.tagID}),
	)
}

func (u *uploadPutter) Cleanup(st transaction.Storage) error {
	if u.closed {
		return nil
	}

	itemsToDelete := make([]*pushItem, 0)

	di := &dirtyTagItem{TagID: u.tagID}
	err := st.IndexStore().Get(di)
	if err != nil {
		return fmt.Errorf("failed reading dirty tag while cleaning up: %w", err)
	}

	err = st.IndexStore().Iterate(
		storage.Query{
			Factory:       func() storage.Item { return &pushItem{} },
			PrefixAtStart: true,
			Prefix:        fmt.Sprintf("%d", di.Started),
		},
		func(res storage.Result) (bool, error) {
			pi := res.Entry.(*pushItem)
			if pi.TagID == u.tagID {
				itemsToDelete = append(itemsToDelete, pi)
			}
			return false, nil
		},
	)
	if err != nil {
		return fmt.Errorf("failed iterating over push items: %w", err)
	}

	var eg errgroup.Group
	eg.SetLimit(runtime.NumCPU())

	for _, item := range itemsToDelete {
		func(item *pushItem) {
			eg.Go(func() error {
				return st.Run(context.Background(), func(s transaction.Store) error {
					ui := &uploadItem{Address: item.Address, BatchID: item.BatchID}
					return errors.Join(
						s.IndexStore().Delete(ui),
						s.ChunkStore().Delete(context.Background(), item.Address),
						chunkstamp.Delete(s.IndexStore(), uploadScope, item.Address, item.BatchID),
						s.IndexStore().Delete(item),
					)
				})
			})
		}(item)
	}

	return errors.Join(
		eg.Wait(),
		st.Run(context.Background(), func(s transaction.Store) error {
			return s.IndexStore().Delete(&dirtyTagItem{TagID: u.tagID})
		}),
	)
}

// CleanupDirty does a best-effort cleanup of dirty tags. This is called on startup.
func CleanupDirty(st transaction.Storage) error {
	dirtyTags := make([]*dirtyTagItem, 0)

	err := st.IndexStore().Iterate(
		storage.Query{
			Factory: func() storage.Item { return &dirtyTagItem{} },
		},
		func(res storage.Result) (bool, error) {
			di := res.Entry.(*dirtyTagItem)
			dirtyTags = append(dirtyTags, di)
			return false, nil
		},
	)
	if err != nil {
		return fmt.Errorf("failed iterating dirty tags: %w", err)
	}

	for _, di := range dirtyTags {
		err = errors.Join(err, (&uploadPutter{tagID: di.TagID}).Cleanup(st))
	}

	return err
}

// Report is the implementation of the PushReporter interface.
func Report(ctx context.Context, st transaction.Store, chunk swarm.Chunk, state storage.ChunkState) error {

	ui := &uploadItem{Address: chunk.Address(), BatchID: chunk.Stamp().BatchID()}

	indexStore := st.IndexStore()

	err := indexStore.Get(ui)
	if err != nil {
		// because of the nature of the feed mechanism of the uploadstore/pusher, a chunk that is in inflight may be sent more than once to the pusher.
		// this is because the chunks are removed from the queue only when they are synced, not at the start of the upload
		if errors.Is(err, storage.ErrNotFound) {
			return nil
		}

		return fmt.Errorf("failed to read uploadItem %x: %w", ui.BatchID, err)
	}

	ti := &TagItem{TagID: ui.TagID}
	err = indexStore.Get(ti)
	if err != nil {
		return fmt.Errorf("failed getting tag: %w", err)
	}

	switch state {
	case storage.ChunkSent:
		ti.Sent++
	case storage.ChunkStored:
		ti.Stored++
		// also mark it as synced
		fallthrough
	case storage.ChunkSynced:
		ti.Synced++
	case storage.ChunkCouldNotSync:
		break
	}

	err = indexStore.Put(ti)
	if err != nil {
		return fmt.Errorf("failed updating tag: %w", err)
	}

	if state == storage.ChunkSent {
		return nil
	}

	// Once the chunk is stored/synced/failed to sync, it is deleted from the upload store as
	// we no longer need to keep track of this chunk. We also need to cleanup
	// the pushItem.
	pi := &pushItem{
		Timestamp: ui.Uploaded,
		Address:   chunk.Address(),
		BatchID:   chunk.Stamp().BatchID(),
	}

	return errors.Join(
		indexStore.Delete(pi),
		chunkstamp.Delete(indexStore, uploadScope, pi.Address, pi.BatchID),
		st.ChunkStore().Delete(ctx, chunk.Address()),
		indexStore.Delete(ui),
	)
}

var (
	errNextTagIDUnmarshalInvalidSize = errors.New("unmarshal nextTagID: invalid size")
)

// nextTagID is a storage.Item which stores a uint64 value in the store.
type nextTagID uint64

func (nextTagID) Namespace() string { return "upload" }

func (nextTagID) ID() string { return "nextTagID" }

func (n nextTagID) Marshal() ([]byte, error) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(n))
	return buf, nil
}

func (n *nextTagID) Unmarshal(buf []byte) error {
	if len(buf) != 8 {
		return errNextTagIDUnmarshalInvalidSize
	}

	*n = nextTagID(binary.LittleEndian.Uint64(buf))
	return nil
}

func (n *nextTagID) Clone() storage.Item {
	if n == nil {
		return nil
	}
	ni := *n
	return &ni
}

func (n nextTagID) String() string {
	return storageutil.JoinFields(n.Namespace(), n.ID())
}

// NextTag returns the next tag ID to be used. It reads the last used ID and
// increments it by 1. This method needs to be called under lock by user as there
// is no guarantee for parallel updates.
func NextTag(st storage.IndexStore) (TagItem, error) {
	var (
		tagID nextTagID
		tag   TagItem
	)
	err := st.Get(&tagID)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return tag, err
	}

	tagID++
	err = st.Put(&tagID)
	if err != nil {
		return tag, err
	}

	tag.TagID = uint64(tagID)
	tag.StartedAt = now().UnixNano()

	return tag, st.Put(&tag)
}

// TagInfo returns the TagItem for this particular tagID.
func TagInfo(st storage.Reader, tagID uint64) (TagItem, error) {
	ti := TagItem{TagID: tagID}
	err := st.Get(&ti)
	if err != nil {
		return ti, fmt.Errorf("uploadstore: failed getting tag %d: %w", tagID, err)
	}

	return ti, nil
}

// ListAllTags returns all the TagItems in the store.
func ListAllTags(st storage.Reader) ([]TagItem, error) {
	var tags []TagItem
	err := st.Iterate(storage.Query{
		Factory: func() storage.Item { return new(TagItem) },
	}, func(r storage.Result) (bool, error) {
		tags = append(tags, *r.Entry.(*TagItem))
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("uploadstore: failed to iterate tags: %w", err)
	}

	return tags, nil
}

func IteratePending(ctx context.Context, s transaction.ReadOnlyStore, consumerFn func(chunk swarm.Chunk) (bool, error)) error {
	return s.IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &pushItem{} },
	}, func(r storage.Result) (bool, error) {
		pi := r.Entry.(*pushItem)
		has, err := s.IndexStore().Has(&dirtyTagItem{TagID: pi.TagID})
		if err != nil {
			return true, err
		}
		if has {
			return false, nil
		}
		chunk, err := s.ChunkStore().Get(ctx, pi.Address)
		if err != nil {
			return true, err
		}

		stamp, err := chunkstamp.LoadWithBatchID(s.IndexStore(), uploadScope, chunk.Address(), pi.BatchID)
		if err != nil {
			return true, err
		}

		chunk = chunk.
			WithStamp(stamp).
			WithTagID(uint32(pi.TagID))

		return consumerFn(chunk)
	})
}

// DeleteTag deletes TagItem associated with the given tagID.
func DeleteTag(st storage.Writer, tagID uint64) error {
	if err := st.Delete(&TagItem{TagID: tagID}); err != nil {
		return fmt.Errorf("uploadstore: failed to delete tag %d: %w", tagID, err)
	}
	return nil
}

func IterateAll(st storage.Reader, iterateFn func(item storage.Item) (bool, error)) error {
	return st.Iterate(
		storage.Query{
			Factory: func() storage.Item { return new(uploadItem) },
		},
		func(r storage.Result) (bool, error) {
			ui := r.Entry.(*uploadItem)
			ui.IdFunc = func() string {
				return r.ID
			}
			return iterateFn(ui)
		},
	)
}

func IterateAllTagItems(st storage.Reader, cb func(ti *TagItem) (bool, error)) error {
	return st.Iterate(
		storage.Query{
			Factory: func() storage.Item { return new(TagItem) },
		},
		func(result storage.Result) (bool, error) {
			ti := result.Entry.(*TagItem)
			return cb(ti)
		},
	)
}

// BatchIDForChunk returns the first known batchID for the given chunk address.
func BatchIDForChunk(st storage.Reader, addr swarm.Address) ([]byte, error) {
	var batchID []byte

	err := st.Iterate(
		storage.Query{
			Factory:      func() storage.Item { return new(uploadItem) },
			Prefix:       addr.ByteString(),
			ItemProperty: storage.QueryItemID,
		},
		func(r storage.Result) (bool, error) {
			if len(r.ID) < 32 {
				return false, nil
			}
			batchID = []byte(r.ID[len(r.ID)-32:])
			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}

	if batchID == nil {
		return nil, storage.ErrNotFound
	}

	return batchID, nil
}
