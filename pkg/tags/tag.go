// Copyright 2019 The go-ethereum Authors
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

package tags

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/opentracing/opentracing-go"
)

var (
	errExists = errors.New("already exists")
	errNA     = errors.New("not available yet")
	errNoETA  = errors.New("unable to calculate ETA")
)

// State is the enum type for chunk states
type State = uint32

const (
	TotalChunks State = iota // The total no of chunks for the tag
	StateSplit               // chunk has been processed by filehasher/swarm safe call
	StateStored              // chunk stored locally
	StateSeen                // chunk previously seen
	StateSent                // chunk sent to neighbourhood
	StateSynced              // proof is received; chunk removed from sync db; chunk is available everywhere
)

// Tag represents info on the status of new chunks
type Tag struct {
	Total  int64 // total chunks belonging to a tag
	Split  int64 // number of chunks already processed by splitter for hashing
	Seen   int64 // number of chunks already seen
	Stored int64 // number of chunks already stored locally
	Sent   int64 // number of chunks sent for push syncing
	Synced int64 // number of chunks synced with proof

	Uid       uint32        // a unique identifier for this tag
	Address   swarm.Address // the associated swarm hash for this tag
	StartedAt time.Time     // tag started to calculate ETA

	// end-to-end tag tracing
	ctx        context.Context     // tracing context
	span       opentracing.Span    // tracing root span
	spanOnce   sync.Once           // make sure we close root span only once
	stateStore storage.StateStorer // to persist the tag
	logger     logging.Logger      // logger instance for logging
}

// NewTag creates a new tag, and returns it
func NewTag(ctx context.Context, uid uint32, total int64, tracer *tracing.Tracer, stateStore storage.StateStorer, logger logging.Logger) *Tag {
	t := &Tag{
		Uid:        uid,
		StartedAt:  time.Now(),
		Total:      total,
		stateStore: stateStore,
		logger:     logger,
	}

	// context here is used only to store the root span `new.upload.tag` within Tag,
	// we don't need any type of ctx Deadline or cancellation for this particular ctx
	t.span, _, t.ctx = tracer.StartSpanFromContext(ctx, "new.upload.tag", nil)
	return t
}

// Context accessor
func (t *Tag) Context() context.Context {
	return t.ctx
}

// FinishRootSpan closes the pushsync span of the tags
func (t *Tag) FinishRootSpan() {
	t.spanOnce.Do(func() {
		t.span.Finish()
	})
}

// IncN increments the count for a state
func (t *Tag) IncN(state State, n int64) error {
	var v *int64
	switch state {
	case TotalChunks:
		v = &t.Total
	case StateSplit:
		v = &t.Split
	case StateStored:
		v = &t.Stored
	case StateSeen:
		v = &t.Seen
	case StateSent:
		v = &t.Sent
	case StateSynced:
		v = &t.Synced
	}
	atomic.AddInt64(v, n)

	// check if syncing is over and persist the tag
	if state == StateSynced {
		total := atomic.LoadInt64(&t.Total)
		seen := atomic.LoadInt64(&t.Seen)
		synced := atomic.LoadInt64(&t.Synced)
		totalUnique := total - seen
		if synced >= totalUnique {
			return t.saveTag()
		}
	}
	return nil
}

// Inc increments the count for a state
func (t *Tag) Inc(state State) error {
	return t.IncN(state, 1)
}

// Get returns the count for a state on a tag
func (t *Tag) Get(state State) int64 {
	var v *int64
	switch state {
	case TotalChunks:
		v = &t.Total
	case StateSplit:
		v = &t.Split
	case StateStored:
		v = &t.Stored
	case StateSeen:
		v = &t.Seen
	case StateSent:
		v = &t.Sent
	case StateSynced:
		v = &t.Synced
	}
	return atomic.LoadInt64(v)
}

// GetTotal returns the total count
func (t *Tag) TotalCounter() int64 {
	return atomic.LoadInt64(&t.Total)
}

// WaitTillDone returns without error once the tag is complete
// wrt the state given as argument
// it returns an error if the context is done
func (t *Tag) WaitTillDone(ctx context.Context, s State) error {
	if t.Done(s) {
		return nil
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	for {
		select {
		case <-ticker.C:
			if t.Done(s) {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Done returns true if tag is complete wrt the state given as argument
func (t *Tag) Done(s State) bool {
	n, total, err := t.Status(s)
	return err == nil && n == total
}

// DoneSplit sets total count to SPLIT count and sets the associated swarm hash for this tag
// is meant to be called when splitter finishes for input streams of unknown size
func (t *Tag) DoneSplit(address swarm.Address) (int64, error) {
	total := atomic.LoadInt64(&t.Split)
	atomic.StoreInt64(&t.Total, total)

	if !address.Equal(swarm.ZeroAddress) {
		t.Address = address
	}

	// persist the tag
	err := t.saveTag()
	if err != nil {
		return 0, err
	}
	return total, nil
}

// Status returns the value of state and the total count
func (t *Tag) Status(state State) (int64, int64, error) {
	count, seen, total := t.Get(state), atomic.LoadInt64(&t.Seen), atomic.LoadInt64(&t.Total)
	if total == 0 {
		return count, total, errNA
	}
	switch state {
	case StateSplit, StateStored, StateSeen:
		return count, total, nil
	case StateSent, StateSynced:
		stored := atomic.LoadInt64(&t.Stored)
		if stored < total {
			return count, total - seen, errNA
		}
		return count, total - seen, nil
	}
	return count, total, errNA
}

// ETA returns the time of completion estimated based on time passed and rate of completion
func (t *Tag) ETA(state State) (time.Time, error) {
	cnt, total, err := t.Status(state)
	if err != nil {
		return time.Time{}, err
	}
	if cnt == 0 || total == 0 {
		return time.Time{}, errNoETA
	}
	diff := time.Since(t.StartedAt)
	dur := time.Duration(total) * diff / time.Duration(cnt)
	return t.StartedAt.Add(dur), nil
}

// MarshalBinary marshals the tag into a byte slice
func (tag *Tag) MarshalBinary() (data []byte, err error) {
	buffer := make([]byte, 4)
	binary.BigEndian.PutUint32(buffer, tag.Uid)
	encodeInt64Append(&buffer, atomic.LoadInt64(&tag.Total))
	encodeInt64Append(&buffer, atomic.LoadInt64(&tag.Split))
	encodeInt64Append(&buffer, atomic.LoadInt64(&tag.Seen))
	encodeInt64Append(&buffer, atomic.LoadInt64(&tag.Stored))
	encodeInt64Append(&buffer, atomic.LoadInt64(&tag.Sent))
	encodeInt64Append(&buffer, atomic.LoadInt64(&tag.Synced))

	intBuffer := make([]byte, 8)

	n := binary.PutVarint(intBuffer, tag.StartedAt.Unix())
	buffer = append(buffer, intBuffer[:n]...)

	n = binary.PutVarint(intBuffer, int64(len(tag.Address.Bytes())))
	buffer = append(buffer, intBuffer[:n]...)
	buffer = append(buffer, tag.Address.Bytes()...)

	return buffer, nil
}

// UnmarshalBinary unmarshals a byte slice into a tag
func (tag *Tag) UnmarshalBinary(buffer []byte) error {
	if len(buffer) < 13 {
		return errors.New("buffer too short")
	}
	tag.Uid = binary.BigEndian.Uint32(buffer)
	buffer = buffer[4:]

	atomic.AddInt64(&tag.Total, decodeInt64Splice(&buffer))
	atomic.AddInt64(&tag.Split, decodeInt64Splice(&buffer))
	atomic.AddInt64(&tag.Seen, decodeInt64Splice(&buffer))
	atomic.AddInt64(&tag.Stored, decodeInt64Splice(&buffer))
	atomic.AddInt64(&tag.Sent, decodeInt64Splice(&buffer))
	atomic.AddInt64(&tag.Synced, decodeInt64Splice(&buffer))

	t, n := binary.Varint(buffer)
	tag.StartedAt = time.Unix(t, 0)
	buffer = buffer[n:]

	t, n = binary.Varint(buffer)
	buffer = buffer[n:]
	if t > 0 {
		tag.Address = swarm.NewAddress(buffer[:t])
	}

	return nil
}

func encodeInt64Append(buffer *[]byte, val int64) {
	intBuffer := make([]byte, 8)
	n := binary.PutVarint(intBuffer, val)
	*buffer = append(*buffer, intBuffer[:n]...)
}

func decodeInt64Splice(buffer *[]byte) int64 {
	val, n := binary.Varint((*buffer))
	*buffer = (*buffer)[n:]
	return val
}

// saveTag update the tag in the state store
func (tag *Tag) saveTag() error {
	key := getKey(tag.Uid)
	value, err := tag.MarshalBinary()
	if err != nil {
		return err
	}

	if tag.stateStore != nil {
		err = tag.stateStore.Put(key, value)
		if err != nil {
			return err
		}
	}
	return nil
}

func getKey(uid uint32) string {
	return fmt.Sprintf("tags_%d", uid)
}
