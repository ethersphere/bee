// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// ChallengeSize is the length of the per-stream challenge issued in Ack.
const ChallengeSize = 32

// MaxRosterSize is the most addresses a ROSTER chunk can name. On the wire an
// address takes its 20 bytes, a field tag and a length: 22 bytes, of the 4096 a
// chunk's payload holds.
const MaxRosterSize = swarm.ChunkSize / (1 + 1 + crypto.AddressSize)

// servicePrefix domain-separates the session feeds of every kind but DATA.
var servicePrefix = []byte("bps-service:v1")

var (
	// ErrInvalidMessage is a frame whose fields no chunk can make right.
	ErrInvalidMessage = errors.New("bps: invalid message")
	// ErrInvalidSOC is a chunk that does not have the id derived from its frame,
	// or does not validate as its owner's single-owner chunk.
	ErrInvalidSOC = errors.New("bps: invalid soc")
	// ErrInvalidRoster is a ROSTER chunk whose payload this node will not send:
	// one that does not decode as a Roster of 20-byte addresses, or that names
	// the admin, or an address twice.
	ErrInvalidRoster = errors.New("bps: invalid roster")
)

// Kind is what a chunk on the wire is.
type Kind int

const (
	KindUnspecified Kind = iota
	KindData             // an update of the stream's session feed
	// KindAuth is the empty chunk at index 0 of the session's AUTH feed: it authenticates
	// a pending stream, or upgrades in place a spectator stream on which a roster naming
	// its identity was delivered; it is a heartbeat on a publisher stream, and is never
	// delivered.
	KindAuth
	// KindEOS is the empty chunk at index 0 of the admin's EOS feed: the channel is ended.
	KindEOS
	// KindRoster is the admin's publisher set as of its index; its payload is a Roster.
	KindRoster
)

func (k Kind) proto() pb.Kind {
	switch k {
	case KindData:
		return pb.Kind_DATA
	case KindAuth:
		return pb.Kind_AUTH
	case KindEOS:
		return pb.Kind_EOS
	case KindRoster:
		return pb.Kind_ROSTER
	default:
		return pb.Kind_KIND_UNSPECIFIED
	}
}

// SessionTopic returns the topic of the session feed of the given kind:
// keccak256(prefix | topic | challenge), with the prefix empty for DATA and
// "bps-service:v1" | kind for any other kind.
func SessionTopic(kind Kind, topic, challenge []byte) ([]byte, error) {
	return sessionTopic(kind.proto(), topic, challenge)
}

// ID returns the single-owner chunk id of the session feed update at index:
// keccak256(SessionTopic(kind, topic, challenge) | index), index big-endian.
func ID(kind Kind, topic, challenge []byte, index uint64) ([]byte, error) {
	return id(kind.proto(), topic, challenge, index)
}

func sessionTopic(kind pb.Kind, topic, challenge []byte) ([]byte, error) {
	var prefix []byte
	if kind != pb.Kind_DATA {
		prefix = append(append([]byte{}, servicePrefix...), byte(kind))
	}
	return crypto.LegacyKeccak256(append(append(prefix, topic...), challenge...))
}

func id(kind pb.Kind, topic, challenge []byte, index uint64) ([]byte, error) {
	topicS, err := sessionTopic(kind, topic, challenge)
	if err != nil {
		return nil, err
	}
	return crypto.LegacyKeccak256(binary.BigEndian.AppendUint64(topicS, index))
}

// isServiceKind reports whether the kind is one of the admin's service kinds,
// whose owner is forced to the principal.
func isServiceKind(kind pb.Kind) bool {
	return kind == pb.Kind_EOS || kind == pb.Kind_ROSTER
}

// isEmptyKind reports whether the kind is an empty chunk at index 0 of its feed.
func isEmptyKind(kind pb.Kind) bool {
	return kind == pb.Kind_AUTH || kind == pb.Kind_EOS
}

// parseFrame runs the checks that need no owner — the challenge length, the
// index, the id the frame fields derive, that an AUTH or EOS is empty — and
// returns the frame's chunk, parsed as a single-owner chunk.
func parseFrame(f *pb.Broadcast, topic []byte) (*soc.SOC, error) {
	if len(f.Challenge) != ChallengeSize {
		return nil, fmt.Errorf("challenge length %d: %w", len(f.Challenge), ErrInvalidMessage)
	}
	// the last index is one no feed has: the cursor, set to index+1, would overflow
	if f.Index == math.MaxUint64 {
		return nil, fmt.Errorf("index %d would overflow the feed's cursor: %w", f.Index, ErrInvalidMessage)
	}
	// a session has one AUTH chunk, and a channel one EOS, at index 0 of its feed
	if isEmptyKind(f.Kind) && f.Index != 0 {
		return nil, fmt.Errorf("%s chunk at index %d: %w", f.Kind, f.Index, ErrInvalidSOC)
	}
	want, err := id(f.Kind, topic, f.Challenge, f.Index)
	if err != nil {
		return nil, err
	}
	c, err := soc.FromChunk(swarm.NewChunk(swarm.ZeroAddress, f.Soc))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSOC, err)
	}
	if !bytes.Equal(c.ID(), want) {
		return nil, fmt.Errorf("id not derived from frame: %w", ErrInvalidSOC)
	}
	if isEmptyKind(f.Kind) {
		// span 0, no payload
		if len(f.Soc) != swarm.SocMinChunkSize || binary.LittleEndian.Uint64(c.WrappedChunk().Data()) != 0 {
			return nil, fmt.Errorf("%s chunk not empty: %w", f.Kind, ErrInvalidSOC)
		}
	}
	return c, nil
}

// verify checks the frame as parseFrame does, and that its chunk is owner's:
// the chunk is then valid at keccak256(id | owner).
func verify(f *pb.Broadcast, topic, owner []byte) (*soc.SOC, error) {
	c, err := parseFrame(f, topic)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(c.OwnerAddress(), owner) {
		return nil, fmt.Errorf("not the owner's soc: %w", ErrInvalidSOC)
	}
	return c, nil
}

// payload returns what a single-owner chunk carries, without the span.
func payload(c *soc.SOC) []byte {
	return c.WrappedChunk().Data()[swarm.SpanSize:]
}

// decodeRosterMsg returns the publisher set in a ROSTER chunk's payload, and
// false if it does not decode as a Roster of 20-byte addresses: the admin's
// mistake, which a receiver drops.
func decodeRosterMsg(b []byte) ([][]byte, bool) {
	var r pb.Roster
	if err := r.Unmarshal(b); err != nil {
		return nil, false
	}
	for _, a := range r.Publishers {
		if len(a) != crypto.AddressSize {
			return nil, false
		}
	}
	return r.Publishers, true
}
