// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package feeds implements generic interfaces and methods for time-based feeds
// indexing schemes are implemented in subpackages
// - epochs
// - sequence
package feeds

import (
	"encoding"
	"errors"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var ErrFeedTypeNotFound = errors.New("no such feed type")

// Factory creates feed lookups for different types of feeds.
type Factory interface {
	NewLookup(Type, *Feed) (Lookup, error)
}

type Type int

const (
	Sequence Type = iota
	Epoch
)

func (t Type) String() string {
	switch t {
	case Sequence:
		return "Sequence"
	case Epoch:
		return "Epoch"
	default:
		return ""
	}
}

func (t *Type) FromString(s string) error {
	switch s = strings.ToLower(s); s {
	case "sequence":
		*t = Sequence
	case "epoch":
		*t = Epoch
	default:
		return ErrFeedTypeNotFound
	}
	return nil
}

type id struct {
	topic []byte
	index []byte
}

var _ encoding.BinaryMarshaler = (*id)(nil)

func (i *id) MarshalBinary() ([]byte, error) {
	return crypto.LegacyKeccak256(append(i.topic, i.index...))
}

// Feed is representing an epoch based feed
type Feed struct {
	Topic []byte
	Owner common.Address
}

// New constructs an epoch based feed from a human readable topic and an ether address
func New(topic string, owner common.Address) (*Feed, error) {
	th, err := crypto.LegacyKeccak256([]byte(topic))
	if err != nil {
		return nil, err
	}
	return &Feed{th, owner}, nil
}

// Index is the interface for feed implementations
type Index interface {
	encoding.BinaryMarshaler
}

// Update represents an update instance of a feed, i.e., pairing of a Feed with an Epoch
type Update struct {
	*Feed
	index Index
}

// Update called on a feed with an index and returns an Update
func (f *Feed) Update(index Index) *Update {
	return &Update{f, index}
}

func NewUpdate(f *Feed, idx Index, timestamp int64, payload []byte, sig []byte) (swarm.Chunk, error) {
	id, err := f.Update(idx).Id()
	if err != nil {
		return nil, fmt.Errorf("update: %w", err)
	}
	cac, err := toChunk(uint64(timestamp), payload)
	if err != nil {
		return nil, fmt.Errorf("toChunk: %w", err)
	}

	ch, err := soc.NewSignedChunk(id, cac, f.Owner.Bytes(), sig)
	if err != nil {
		return nil, fmt.Errorf("new chunk: %w", err)
	}
	if !soc.Valid(ch) {
		return nil, storage.ErrInvalidChunk
	}
	return ch, nil
}

// Id calculates the identifier if a  feed update to be used in single owner chunks
func (u *Update) Id() ([]byte, error) {
	index, err := u.index.MarshalBinary()
	if err != nil {
		return nil, err
	}
	i := &id{u.Topic, index}
	return i.MarshalBinary()
}

// Address calculates the soc address of a feed update
func (u *Update) Address() (swarm.Address, error) {
	var addr swarm.Address
	i, err := u.Id()
	if err != nil {
		return addr, err
	}
	owner, err := soc.NewOwner(u.Owner[:])
	if err != nil {
		return addr, err
	}
	return soc.CreateAddress(i, owner)
}
