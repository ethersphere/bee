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

// ChallengeSize is the length of the challenge the broker sends in its Ack.
const ChallengeSize = 32

// MaxRosterSize is the most addresses a ROSTER chunk can hold. Each address
// takes 22 bytes on the wire: a tag, a length and 20 bytes.
const MaxRosterSize = swarm.ChunkSize / (1 + 1 + crypto.AddressSize)

// servicePrefix keeps the topics of non-DATA kinds apart from DATA topics.
var servicePrefix = []byte("bps-service:v1")

var (
	// ErrInvalidMessage means the frame fields are malformed.
	ErrInvalidMessage = errors.New("bps: invalid message")
	// ErrInvalidSOC means the chunk's id does not match the frame, or the
	// chunk is not a valid single-owner chunk of the expected owner.
	ErrInvalidSOC = errors.New("bps: invalid soc")
	// ErrInvalidRoster means the roster payload does not decode, names the
	// admin, or names an address twice.
	ErrInvalidRoster = errors.New("bps: invalid roster")
)

// Kind is the type of a chunk on the wire.
type Kind int

const (
	KindUnspecified Kind = iota
	KindData             // a publisher's content
	// KindAuth is an empty chunk at index 0. It makes a stream a publisher.
	// On a publisher stream it is a heartbeat. It is never delivered.
	KindAuth
	// KindEOS is an empty chunk at index 0 from the admin. It ends the channel.
	KindEOS
	// KindRoster is the admin's list of publishers. Its payload is a Roster.
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

// SessionTopic returns keccak256(prefix | topic | challenge), the feed topic
// for chunks of the given kind. The prefix is empty for DATA, and
// "bps-service:v1" followed by the kind byte otherwise.
func SessionTopic(kind Kind, topic, challenge []byte) ([]byte, error) {
	var prefix []byte
	if kind != KindData {
		prefix = append(append([]byte{}, servicePrefix...), byte(kind.proto()))
	}
	return crypto.LegacyKeccak256(append(append(prefix, topic...), challenge...))
}

// ID returns the single-owner chunk id for the given index:
// keccak256(SessionTopic(kind, topic, challenge) | index), index big-endian.
func ID(kind Kind, topic, challenge []byte, index uint64) ([]byte, error) {
	topicS, err := SessionTopic(kind, topic, challenge)
	if err != nil {
		return nil, fmt.Errorf("session topic: %w", err)
	}
	return crypto.LegacyKeccak256(binary.BigEndian.AppendUint64(topicS, index))
}

// isServiceKind reports whether only the principal may sign the kind.
func isServiceKind(kind pb.Kind) bool {
	return kind == pb.Kind_EOS || kind == pb.Kind_ROSTER
}

// isEmptyKind reports whether the kind must be an empty chunk at index 0.
func isEmptyKind(kind pb.Kind) bool {
	return kind == pb.Kind_AUTH || kind == pb.Kind_EOS
}

// parseFrame parses the frame's chunk and runs the checks that do not need
// the owner. It checks the challenge length, the index, the chunk id, and that
// AUTH and EOS are empty.
func parseFrame(f *pb.Broadcast, topic []byte) (*soc.SOC, error) {
	if len(f.Challenge) != ChallengeSize {
		return nil, fmt.Errorf("challenge length %d: %w", len(f.Challenge), ErrInvalidMessage)
	}
	// the cursor is set to index+1, which would overflow
	if f.Index == math.MaxUint64 {
		return nil, fmt.Errorf("index %d would overflow the feed's cursor: %w", f.Index, ErrInvalidMessage)
	}
	if isEmptyKind(f.Kind) && f.Index != 0 {
		return nil, fmt.Errorf("%s chunk at index %d: %w", f.Kind, f.Index, ErrInvalidSOC)
	}
	// Kind and pb.Kind share values
	want, err := ID(Kind(f.Kind), topic, f.Challenge, f.Index)
	if err != nil {
		return nil, fmt.Errorf("id: %w", err)
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

// verify runs parseFrame and checks that owner signed the chunk.
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

// payload returns the chunk's data without the span.
func payload(c *soc.SOC) []byte {
	return c.WrappedChunk().Data()[swarm.SpanSize:]
}

// decodeRosterMsg returns the addresses in a ROSTER payload. It returns false
// if the payload is not a Roster of 20-byte addresses.
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
