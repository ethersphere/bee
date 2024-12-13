// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package feeds

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/v2/pkg/soc"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var errNotLegacyPayload = errors.New("feed update is not in the legacy payload structure")

// Lookup is the interface for time based feed lookup
type Lookup interface {
	At(ctx context.Context, at int64, after uint64) (chunk swarm.Chunk, currentIndex, nextIndex Index, err error)
}

// Getter encapsulates a chunk Getter getter and a feed and provides non-concurrent lookup methods
type Getter struct {
	getter storage.Getter
	*Feed
}

// NewGetter constructs a feed Getter
func NewGetter(getter storage.Getter, feed *Feed) *Getter {
	return &Getter{getter, feed}
}

// Latest looks up the latest update of the feed
// after is a unix time hint of the latest known update
func Latest(ctx context.Context, l Lookup, after uint64) (swarm.Chunk, error) {
	c, _, _, err := l.At(ctx, time.Now().Unix(), after)
	return c, err
}

// Get creates an update of the underlying feed at the given epoch
// and looks it up in the chunk Getter based on its address
func (f *Getter) Get(ctx context.Context, i Index) (swarm.Chunk, error) {
	addr, err := f.Feed.Update(i).Address()
	if err != nil {
		return nil, err
	}
	return f.getter.Get(ctx, addr)
}

func GetWrappedChunk(ctx context.Context, getter storage.Getter, ch swarm.Chunk) (swarm.Chunk, error) {
	wc, err := FromChunk(ch)
	if err != nil {
		return nil, err
	}
	// try to split the timestamp and reference
	// possible values right now:
	// unencrypted ref: span+timestamp+ref => 8+8+32=48
	// encrypted ref: span+timestamp+ref+decryptKey => 8+8+64=80
	ref, err := legacyPayload(wc)
	if err != nil {
		if errors.Is(err, errNotLegacyPayload) {
			return wc, nil
		}
		return nil, err
	}
	wc, err = getter.Get(ctx, ref)
	if err != nil {
		return nil, err
	}

	return wc, nil
}

// FromChunk parses out the wrapped chunk
func FromChunk(ch swarm.Chunk) (swarm.Chunk, error) {
	s, err := soc.FromChunk(ch)
	if err != nil {
		return nil, fmt.Errorf("soc unmarshal: %w", err)
	}
	return s.WrappedChunk(), nil
}

// legacyPayload returns back the referenced chunk and datetime from the legacy feed payload
func legacyPayload(wrappedChunk swarm.Chunk) (swarm.Address, error) {
	cacData := wrappedChunk.Data()
	if !(len(cacData) == 16+swarm.HashSize || len(cacData) == 16+swarm.HashSize*2) {
		return swarm.ZeroAddress, errNotLegacyPayload
	}

	return swarm.NewAddress(cacData[16:]), nil
}
