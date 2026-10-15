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

// servicePrefix domain-separates the session feeds of every kind but DATA.
var servicePrefix = []byte("bps-service:v1")

// errInvalidSOC is returned for a chunk that does not have the id derived
// from its frame, or does not validate as the principal's single-owner chunk.
var errInvalidSOC = errors.New("bps: invalid soc")

// Kind is what a chunk on the wire is.
type Kind int

const (
	KindUnspecified Kind = iota
	KindData             // an update of the stream's session feed
	// KindAuth is the empty chunk at index 0 of the session's AUTH feed: it authenticates
	// a pending stream, is a heartbeat on a publisher stream, and is never delivered.
	KindAuth
)

func (k Kind) proto() pb.Kind {
	switch k {
	case KindData:
		return pb.Kind_DATA
	case KindAuth:
		return pb.Kind_AUTH
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

// Verify checks that the frame fields kind, challenge and index give the id of
// the chunk, and that the chunk validates as the principal's single-owner chunk
// at keccak256(id | principal). An AUTH chunk must moreover be at index 0 and empty.
func Verify(kind Kind, challenge []byte, index uint64, chunk, topic, principal []byte) error {
	return verify(&pb.Broadcast{Soc: chunk, Kind: kind.proto(), Challenge: challenge, Index: index}, topic, principal)
}

func verify(f *pb.Broadcast, topic, principal []byte) error {
	if len(f.Challenge) != ChallengeSize {
		return fmt.Errorf("challenge length %d: %w", len(f.Challenge), errInvalidSOC)
	}
	// the last index is one no feed has: the cursor, set to index+1, never wraps
	if f.Index == math.MaxUint64 {
		return fmt.Errorf("index %d: %w", f.Index, errInvalidSOC)
	}
	// a session has one AUTH chunk, at index 0 of its AUTH feed
	if f.Kind == pb.Kind_AUTH && f.Index != 0 {
		return fmt.Errorf("auth chunk at index %d: %w", f.Index, errInvalidSOC)
	}
	want, err := id(f.Kind, topic, f.Challenge, f.Index)
	if err != nil {
		return err
	}
	if len(f.Soc) < swarm.HashSize || !bytes.Equal(f.Soc[:swarm.HashSize], want) {
		return fmt.Errorf("id not derived from frame: %w", errInvalidSOC)
	}
	addr, err := soc.CreateAddress(want, principal)
	if err != nil {
		return err
	}
	if !soc.Valid(swarm.NewChunk(addr, f.Soc)) {
		return fmt.Errorf("not the principal's soc: %w", errInvalidSOC)
	}
	if f.Kind == pb.Kind_AUTH {
		// span 0, no payload
		if len(f.Soc) != swarm.HashSize+swarm.SocSignatureSize+swarm.SpanSize ||
			binary.LittleEndian.Uint64(f.Soc[swarm.HashSize+swarm.SocSignatureSize:]) != 0 {
			return fmt.Errorf("auth chunk not empty: %w", errInvalidSOC)
		}
	}
	return nil
}
