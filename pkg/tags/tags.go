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

// Package tags provides the implementation for
// upload progress tracking.
package tags

import (
	"context"
	"errors"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	maxPage      = 1000 // hard limit of page size
	tagKeyPrefix = "tags_"
)

var (
	TagUidFunc  = rand.Uint32
	ErrNotFound = errors.New("tag not found")
)

// Tags hold tag information indexed by a unique random uint32
type Tags struct {
	tags   *sync.Map
	logger logging.Logger
}

// NewTags creates a tags object
func NewTags(logger logging.Logger) *Tags {
	return &Tags{
		tags:   &sync.Map{},
		logger: logger,
	}
}

// Create creates a new tag, stores it by the UID and returns it
// it returns an error if the tag with this UID already exists
func (ts *Tags) Create(total int64) (*Tag, error) {
	t := NewTag(context.Background(), TagUidFunc(), total, nil, ts.logger)

	if _, loaded := ts.tags.LoadOrStore(t.Uid, t); loaded {
		return nil, errExists
	}

	return t, nil
}

// All returns all existing tags in Tags' sync.Map
// Note that tags are returned in no particular order
func (ts *Tags) All() (t []*Tag) {
	ts.tags.Range(func(k, v interface{}) bool {
		t = append(t, v.(*Tag))
		return true
	})

	return t
}

// Get returns the underlying tag for the uid or an error if not found
func (ts *Tags) Get(uid uint32) (*Tag, error) {
	t, ok := ts.tags.Load(uid)
	if !ok {
		return nil, ErrNotFound
	}
	return t.(*Tag), nil
}

// GetByAddress returns the latest underlying tag for the address or an error if not found
func (ts *Tags) GetByAddress(address swarm.Address) (*Tag, error) {
	var t *Tag
	var lastTime time.Time
	ts.tags.Range(func(key interface{}, value interface{}) bool {
		rcvdTag := value.(*Tag)
		if rcvdTag.Address.Equal(address) && rcvdTag.StartedAt.After(lastTime) {
			t = rcvdTag
			lastTime = rcvdTag.StartedAt
		}
		return true
	})

	if t == nil {
		return nil, ErrNotFound
	}
	return t, nil
}

// Range exposes sync.Map's iterator
func (ts *Tags) Range(fn func(k, v interface{}) bool) {
	ts.tags.Range(fn)
}

func (ts *Tags) Delete(k interface{}) {
	ts.tags.Delete(k)
}

func (ts *Tags) ListAll(ctx context.Context, offset, limit int) (t []*Tag, err error) {
	if limit > maxPage {
		limit = maxPage
	}

	// range sync.Map first
	allTags := ts.All()
	sort.Slice(allTags, func(i, j int) bool { return allTags[i].Uid < allTags[j].Uid })
	for _, tag := range allTags {
		if offset > 0 {
			offset--
			continue
		}

		t = append(t, tag)

		limit--

		if limit == 0 {
			break
		}
	}

	if limit == 0 {
		return
	}

	return t, err
}
