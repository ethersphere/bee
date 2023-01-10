// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package upload

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"path"
	"strconv"
	"time"

	"github.com/ethersphere/bee/pkg/localstorev2/internal"
	storage "github.com/ethersphere/bee/pkg/storagev2"
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

var _ storage.Item = (*pushItem)(nil)

// pushItem is an store.Item that represents data relevant to push.
// The key is a combination of Timestamp and Address, where the
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

// String implements the fmt.Stringer interface.
func (i pushItem) String() string {
	return path.Join(i.Namespace(), i.ID())
}

var (
	// errTagIDAddressItemUnmarshalInvalidSize is returned when trying
	// to unmarshal buffer that is not of size tagItemSize.
	errTagItemUnmarshalInvalidSize = errors.New("unmarshal tagItem: invalid size")
)

// tagItemSize is the size of a marshaled tagItem.
const tagItemSize = swarm.HashSize + 7*8

var _ storage.Item = (*tagItem)(nil)

// tagItem is an store.Item that stores addresses of already seen chunks.
type tagItem struct {
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
func (i tagItem) ID() string {
	return strconv.FormatUint(i.TagID, 10)
}

// Namespace implements the storage.Item interface.
func (i tagItem) Namespace() string {
	return "tagItem"
}

// Marshal implements the storage.Item interface.
func (i tagItem) Marshal() ([]byte, error) {
	buf := make([]byte, tagItemSize)
	binary.LittleEndian.PutUint64(buf, i.TagID)
	binary.LittleEndian.PutUint64(buf[8:], i.Split)
	binary.LittleEndian.PutUint64(buf[16:], i.Seen)
	binary.LittleEndian.PutUint64(buf[24:], i.Stored)
	binary.LittleEndian.PutUint64(buf[32:], i.Sent)
	binary.LittleEndian.PutUint64(buf[40:], i.Synced)
	copy(buf[48:], internal.AddressBytesOrZero(i.Address))
	binary.LittleEndian.PutUint64(buf[48+swarm.HashSize:], uint64(i.StartedAt))
	return buf, nil
}

// Unmarshal implements the storage.Item interface.
// If the buffer is not of size tagItemSize, an error is returned.
func (i *tagItem) Unmarshal(bytes []byte) error {
	if len(bytes) != tagItemSize {
		return errTagItemUnmarshalInvalidSize
	}
	ni := new(tagItem)
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

// String implements the fmt.Stringer interface.
func (i tagItem) String() string {
	return path.Join(i.Namespace(), i.ID())
}

var (
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
	return string(i.BatchID)
}

// Namespace implements the storage.Item interface.
func (i uploadItem) Namespace() string {
	return path.Join("UploadItem", i.Address.ByteString())
}

// Marshal implements the storage.Item interface.
// If the Address is zero, an error is returned.
func (i uploadItem) Marshal() ([]byte, error) {
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
	ni := new(uploadItem)
	ni.TagID = binary.LittleEndian.Uint64(bytes[:8])
	ni.Uploaded = int64(binary.LittleEndian.Uint64(bytes[8:16]))
	ni.Synced = int64(binary.LittleEndian.Uint64(bytes[16:]))
	*i = *ni
	return nil
}

// String implements the fmt.Stringer interface.
func (i uploadItem) String() string {
	return path.Join(i.Namespace(), i.ID())
}

type uploadPutter struct {
	tag *tagItem
	s   internal.Storage
}

func NewPutter(s internal.Storage, tagId uint64) internal.PutterCloserWithReference {
	return &uploadPutter{
		tag: &tagItem{TagID: tagId, StartedAt: now().Unix()},
		s:   s,
	}
}

// Put operation will do the following:
// 1. If upload store has already seen this chunk, it will update the tag and return
// 2. For a new chunk it will add:
//    a. uploadItem entry to keep track of this chunk.
//    b. pushItem entry to make it available for PushSubscriber
//    c. add chunk to the chunkstore till it is synced
func (u *uploadPutter) Put(ctx context.Context, chunk swarm.Chunk) error {
	u.tag.Split++

	// Check if upload store has already seen this chunk
	ui := &uploadItem{
		Address: chunk.Address(),
		BatchID: chunk.Stamp().BatchID(),
	}
	switch exists, err := u.s.Store().Has(ui); {
	case err != nil:
		return fmt.Errorf("store has item %q call failed: %w", ui, err)
	case exists:
		u.tag.Seen++
		return nil
	}

	if err := u.s.ChunkStore().Put(ctx, chunk); err != nil {
		return fmt.Errorf("chunk store put chunk %q call failed: %w", chunk.Address(), err)
	}

	ui.Uploaded = now().Unix()
	ui.TagID = u.tag.TagID

	if err := u.s.Store().Put(ui); err != nil {
		return fmt.Errorf("store put item %q call failed: %w", ui, err)
	}

	pi := &pushItem{
		Timestamp: ui.Uploaded,
		Address:   chunk.Address(),
		BatchID:   chunk.Stamp().BatchID(),
		TagID:     u.tag.TagID,
	}
	if err := u.s.Store().Put(pi); err != nil {
		return fmt.Errorf("store put item %q call failed: %w", pi, err)
	}

	return nil
}

// Close provides the CloseWithReference interface where the session can be associated
// with a swarm reference. This can be useful while keeping track of uploads through
// the tags. It will update the tag. This will be filled with the Split and Seen count
// by the Putter.
func (u *uploadPutter) Close(addr swarm.Address) error {
	if !addr.IsZero() {
		u.tag.Address = addr.Clone()
	}

	err := u.s.Store().Put(u.tag)
	if err != nil {
		return fmt.Errorf("failed storing tag: %w", err)
	}

	return nil
}

type pushReporter struct {
	s internal.Storage
}

func NewPushReporter(s internal.Storage) *pushReporter {
	return &pushReporter{s: s}
}

// Sent is used by the pusher component to notify about successful push of chunk from
// the node. A chunk could be retried on failure so, this sent count is maintained to
// understand how many attempts were made by the node while pushing. The attempts are
// registered only when an actual request was sent from this node.
func (p *pushReporter) Sent(chunk swarm.Chunk) error {
	ui := &uploadItem{
		Address: chunk.Address(),
		BatchID: chunk.Stamp().BatchID(),
	}

	err := p.s.Store().Get(ui)
	if err != nil {
		return fmt.Errorf("failed to read uploadItem %s: %w", ui, err)
	}

	ti := &tagItem{
		TagID: ui.TagID,
	}

	err = p.s.Store().Get(ti)
	if err != nil {
		return fmt.Errorf("failed getting tag: %w", err)
	}

	ti.Sent++
	err = p.s.Store().Put(ti)
	if err != nil {
		return fmt.Errorf("failed updating tag: %w", err)
	}

	return nil
}

// Synced is used by the pusher component to notify that the chunk is synced to the
// network. This is reported when a valid receipt was received after the chunk was
// pushed. Once the chunk is synced, the chunk is deleted from the upload store as
// we no longer need to keep track of this chunk. It could potentially be saved by
// some other component. The uploadItem is kept around to have information about
// the chunk, which can be used for debug functionality. These need to have a separate
// cleanup process.
func (p *pushReporter) Synced(chunk swarm.Chunk) error {
	ui := &uploadItem{
		Address: chunk.Address(),
		BatchID: chunk.Stamp().BatchID(),
	}

	err := p.s.Store().Get(ui)
	if err != nil {
		return fmt.Errorf("failed to read upload item %s: %w", ui, err)
	}

	pi := &pushItem{
		Timestamp: ui.Uploaded,
		Address:   chunk.Address(),
		BatchID:   chunk.Stamp().BatchID(),
	}

	err = p.s.Store().Get(pi)
	if err != nil {
		return fmt.Errorf("failed reading pushItem: %w", err)
	}

	err = p.s.Store().Delete(pi)
	if err != nil {
		return fmt.Errorf("failed deleting pushItem %s: %w", pi, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = p.s.ChunkStore().DeleteWithStamp(ctx, chunk.Address(), chunk.Stamp().BatchID())
	if err != nil {
		return fmt.Errorf("failed deleting chunk %s: %w", chunk.Address(), err)
	}

	ui.Synced = now().Unix()
	err = p.s.Store().Put(ui)
	if err != nil {
		return fmt.Errorf("failed updating uploadItem %s: %w", ui, err)
	}

	ti := &tagItem{
		TagID: ui.TagID,
	}

	err = p.s.Store().Get(ti)
	if err != nil {
		return fmt.Errorf("failed getting tag: %w", err)
	}

	ti.Synced++
	err = p.s.Store().Put(ti)
	if err != nil {
		return fmt.Errorf("failed updating tag: %w", err)
	}
	return nil
}
