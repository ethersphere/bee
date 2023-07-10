// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package upload

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/encryption"
	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/storageutil"
	"github.com/ethersphere/bee/pkg/storer/internal"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/pkg/storer/internal/stampindex"
	"github.com/ethersphere/bee/pkg/swarm"
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

const chunkStampNamespace = "upload"

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
}

// ID implements the storage.Item interface.
func (i uploadItem) ID() string {
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

// stampIndexUploadNamespace represents the
// namespace name of the stamp index for upload.
const stampIndexUploadNamespace = "upload"

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
	mtx    sync.Mutex
	split  uint64
	seen   uint64
	closed bool
}

// NewPutter returns a new chunk putter associated with the tagID.
func NewPutter(s internal.Storage, tagID uint64) (internal.PutterCloserWithReference, error) {
	ti := &TagItem{TagID: tagID}
	has, err := s.IndexStore().Has(ti)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, fmt.Errorf("upload store: tag %d not found: %w", tagID, storage.ErrNotFound)
	}
	err = s.IndexStore().Put(&dirtyTagItem{TagID: tagID, Started: now().UnixNano()})
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
func (u *uploadPutter) Put(ctx context.Context, s internal.Storage, chunk swarm.Chunk) error {
	u.mtx.Lock()
	defer u.mtx.Unlock()

	if u.closed {
		return errPutterAlreadyClosed
	}

	// Check if upload store has already seen this chunk
	ui := &uploadItem{
		Address: chunk.Address(),
		BatchID: chunk.Stamp().BatchID(),
	}
	switch exists, err := s.IndexStore().Has(ui); {
	case err != nil:
		return fmt.Errorf("store has item %q call failed: %w", ui, err)
	case exists:
		u.seen++
		u.split++
		return nil
	}

	switch item, loaded, err := stampindex.LoadOrStore(
		s.IndexStore(),
		s.IndexStore(),
		stampIndexUploadNamespace,
		chunk,
	); {
	case err != nil:
		return fmt.Errorf("load or store stamp index for chunk %v has fail: %w", chunk, err)
	case loaded && item.ChunkIsImmutable:
		return errOverwriteOfImmutableBatch
	case loaded && !item.ChunkIsImmutable:
		prev := binary.BigEndian.Uint64(item.StampTimestamp)
		curr := binary.BigEndian.Uint64(chunk.Stamp().Timestamp())
		if prev >= curr {
			return errOverwriteOfNewerBatch
		}
		err = stampindex.Store(s.IndexStore(), stampIndexUploadNamespace, chunk)
		if err != nil {
			return fmt.Errorf("failed updating stamp index: %w", err)
		}
	}

	u.split++

	if err := s.ChunkStore().Put(ctx, chunk); err != nil {
		return fmt.Errorf("chunk store put chunk %q call failed: %w", chunk.Address(), err)
	}

	if err := chunkstamp.Store(s.IndexStore(), chunkStampNamespace, chunk); err != nil {
		return fmt.Errorf("associate chunk with stamp %q call failed: %w", chunk.Address(), err)
	}

	ui.Uploaded = now().UnixNano()
	ui.TagID = u.tagID

	if err := s.IndexStore().Put(ui); err != nil {
		return fmt.Errorf("store put item %q call failed: %w", ui, err)
	}

	pi := &pushItem{
		Timestamp: ui.Uploaded,
		Address:   chunk.Address(),
		BatchID:   chunk.Stamp().BatchID(),
		TagID:     u.tagID,
	}
	if err := s.IndexStore().Put(pi); err != nil {
		return fmt.Errorf("store put item %q call failed: %w", pi, err)
	}

	return nil
}

// Close provides the CloseWithReference interface where the session can be associated
// with a swarm reference. This can be useful while keeping track of uploads through
// the tags. It will update the tag. This will be filled with the Split and Seen count
// by the Putter.
func (u *uploadPutter) Close(s internal.Storage, addr swarm.Address) error {
	u.mtx.Lock()
	defer u.mtx.Unlock()

	if u.closed {
		return nil
	}

	ti := &TagItem{TagID: u.tagID}
	err := s.IndexStore().Get(ti)
	if err != nil {
		return fmt.Errorf("failed reading tag while closing: %w", err)
	}

	ti.Split += u.split
	ti.Seen += u.seen

	if !addr.IsZero() {
		ti.Address = addr.Clone()
	}

	err = s.IndexStore().Put(ti)
	if err != nil {
		return fmt.Errorf("failed storing tag: %w", err)
	}

	err = s.IndexStore().Delete(&dirtyTagItem{TagID: u.tagID})
	if err != nil {
		return fmt.Errorf("failed deleting dirty tag: %w", err)
	}

	u.closed = true

	return nil
}

func (u *uploadPutter) Cleanup(tx internal.TxExecutor) error {
	u.mtx.Lock()
	defer u.mtx.Unlock()

	if u.closed {
		return nil
	}

	itemsToDelete := make([]*pushItem, 0)

	err := tx.Execute(context.Background(), func(s internal.Storage) error {
		di := &dirtyTagItem{TagID: u.tagID}
		err := s.IndexStore().Get(di)
		if err != nil {
			return fmt.Errorf("failed reading dirty tag while cleaning up: %w", err)
		}

		return s.IndexStore().Iterate(
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
	})
	if err != nil {
		return fmt.Errorf("failed iterating over push items: %w", err)
	}

	batchCnt := 1000
	for i := 0; i < len(itemsToDelete); i += batchCnt {
		err = tx.Execute(context.Background(), func(tx internal.Storage) error {
			end := i + batchCnt
			if end > len(itemsToDelete) {
				end = len(itemsToDelete)
			}
			for _, pi := range itemsToDelete[i:end] {
				_ = remove(tx, pi.Address, pi.BatchID)
				_ = tx.IndexStore().Delete(pi)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed deleting push items: %w", err)
		}
	}

	return tx.Execute(context.Background(), func(tx internal.Storage) error {
		return tx.IndexStore().Delete(&dirtyTagItem{TagID: u.tagID})
	})
}

// Remove removes all the state associated with the given address and batchID.
func remove(st internal.Storage, address swarm.Address, batchID []byte) error {
	ui := &uploadItem{
		Address: address,
		BatchID: batchID,
	}

	err := st.IndexStore().Get(ui)
	if err != nil {
		return fmt.Errorf("failed to read uploadItem %s: %w", ui, err)
	}

	err = st.IndexStore().Delete(ui)
	if err != nil {
		return fmt.Errorf("failed deleting upload item: %w", err)
	}

	if ui.Synced == 0 {
		err = st.ChunkStore().Delete(context.Background(), address)
		if err != nil {
			return fmt.Errorf("failed deleting chunk: %w", err)
		}
		err = chunkstamp.Delete(st.IndexStore(), chunkStampNamespace, address, batchID)
		if err != nil {
			return fmt.Errorf("failed deleting chunk stamp %x: %w", batchID, err)
		}
	}
	return nil
}

// CleanupDirty does a best-effort cleanup of dirty tags. This is called on startup.
func CleanupDirty(tx internal.TxExecutor) error {
	dirtyTags := make([]*dirtyTagItem, 0)

	err := tx.Execute(context.Background(), func(s internal.Storage) error {
		return s.IndexStore().Iterate(
			storage.Query{
				Factory: func() storage.Item { return &dirtyTagItem{} },
			},
			func(res storage.Result) (bool, error) {
				di := res.Entry.(*dirtyTagItem)
				dirtyTags = append(dirtyTags, di)
				return false, nil
			},
		)
	})
	if err != nil {
		return fmt.Errorf("failed iterating dirty tags: %w", err)
	}

	for _, di := range dirtyTags {
		_ = (&uploadPutter{tagID: di.TagID}).Cleanup(tx)
	}

	return nil
}

type pushReporter struct {
	s internal.Storage
}

// NewPushReporter returns a new storage.PushReporter which can be used by the
// pusher component to report chunk state information.
func NewPushReporter(s internal.Storage) storage.PushReporter {
	return &pushReporter{s: s}
}

// Report is the implementation of the PushReporter interface.
func (p *pushReporter) Report(
	ctx context.Context,
	chunk swarm.Chunk,
	state storage.ChunkState,
) error {
	ui := &uploadItem{
		Address: chunk.Address(),
		BatchID: chunk.Stamp().BatchID(),
	}

	err := p.s.IndexStore().Get(ui)
	if err != nil {
		return fmt.Errorf("failed to read uploadItem %s: %w", ui, err)
	}

	ti := &TagItem{
		TagID: ui.TagID,
	}

	err = p.s.IndexStore().Get(ti)
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

	err = p.s.IndexStore().Put(ti)
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

	err = p.s.IndexStore().Delete(pi)
	if err != nil {
		return fmt.Errorf("failed deleting pushItem %s: %w", pi, err)
	}

	err = chunkstamp.Delete(p.s.IndexStore(), chunkStampNamespace, pi.Address, pi.BatchID)
	if err != nil {
		return fmt.Errorf("failed deleting chunk stamp %x: %w", pi.BatchID, err)
	}

	err = p.s.ChunkStore().Delete(ctx, chunk.Address())
	if err != nil {
		return fmt.Errorf("failed deleting chunk %s: %w", chunk.Address(), err)
	}

	ui.Synced = now().UnixNano()
	err = p.s.IndexStore().Put(ui)
	if err != nil {
		return fmt.Errorf("failed updating uploadItem %s: %w", ui, err)
	}

	return nil
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
func NextTag(st storage.Store) (TagItem, error) {
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
func TagInfo(st storage.Store, tagID uint64) (TagItem, error) {
	ti := TagItem{TagID: tagID}
	err := st.Get(&ti)
	if err != nil {
		return ti, fmt.Errorf("uploadstore: failed getting tag %d: %w", tagID, err)
	}

	return ti, nil
}

// ListAllTags returns all the TagItems in the store.
func ListAllTags(st storage.Store) ([]TagItem, error) {
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

func Iterate(ctx context.Context, s internal.Storage, consumerFn func(chunk swarm.Chunk) (bool, error)) error {
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

		stamp, err := chunkstamp.LoadWithBatchID(s.IndexStore(), chunkStampNamespace, chunk.Address(), pi.BatchID)
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
func DeleteTag(st storage.Store, tagID uint64) error {
	if err := st.Delete(&TagItem{TagID: tagID}); err != nil {
		return fmt.Errorf("uploadstore: failed to delete tag %d: %w", tagID, err)
	}
	return nil
}

func IterateAll(st storage.Store, iterateFn func(addr swarm.Address, isSynced bool) (bool, error)) error {
	return st.Iterate(
		storage.Query{
			Factory: func() storage.Item { return new(uploadItem) },
		},
		func(r storage.Result) (bool, error) {
			address := swarm.NewAddress([]byte(r.ID[:32]))
			synced := r.Entry.(*uploadItem).Synced != 0
			return iterateFn(address, synced)
		},
	)
}

// BatchIDForChunk returns the first known batchID for the given chunk address.
func BatchIDForChunk(st storage.Store, addr swarm.Address) ([]byte, error) {
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
