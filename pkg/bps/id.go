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
// from its frame, or does not validate as its owner's single-owner chunk.
var errInvalidSOC = errors.New("bps: invalid soc")

// errInvalidRoster is returned for a ROSTER chunk whose payload does not
// decode as a Roster of 20-byte addresses.
var errInvalidRoster = errors.New("bps: invalid roster")

// Kind is what a chunk on the wire is.
type Kind int

const (
	KindUnspecified Kind = iota
	KindData             // an update of the stream's session feed
	// KindAuth is the empty chunk at index 0 of the session's AUTH feed: it authenticates
	// a pending stream, or a spectator stream a roster names, is a heartbeat on a
	// publisher stream, and is never delivered.
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

// Verify checks that the frame fields kind, challenge and index give the id of
// the chunk, and that the chunk validates as owner's single-owner chunk at
// keccak256(id | owner). An AUTH or EOS chunk must moreover be at index 0 and empty.
func Verify(kind Kind, challenge []byte, index uint64, chunk, topic, owner []byte) error {
	return verify(&pb.Broadcast{Soc: chunk, Kind: kind.proto(), Challenge: challenge, Index: index}, topic, owner)
}

// service reports whether the kind is one of the admin's service kinds, whose
// owner is forced to the principal.
func service(kind pb.Kind) bool {
	return kind == pb.Kind_EOS || kind == pb.Kind_ROSTER
}

// empty reports whether the kind is an empty chunk at index 0 of its feed.
func empty(kind pb.Kind) bool {
	return kind == pb.Kind_AUTH || kind == pb.Kind_EOS
}

// checkFrame runs the checks that need no owner: the challenge length, the
// index, and the id the frame fields derive.
func checkFrame(f *pb.Broadcast, topic []byte) ([]byte, error) {
	if len(f.Challenge) != ChallengeSize {
		return nil, fmt.Errorf("challenge length %d: %w", len(f.Challenge), errInvalidSOC)
	}
	// the last index is one no feed has: the cursor, set to index+1, never wraps
	if f.Index == math.MaxUint64 {
		return nil, fmt.Errorf("index %d: %w", f.Index, errInvalidSOC)
	}
	// a session has one AUTH chunk, and a channel one EOS, at index 0 of its feed
	if empty(f.Kind) && f.Index != 0 {
		return nil, fmt.Errorf("%s chunk at index %d: %w", f.Kind, f.Index, errInvalidSOC)
	}
	want, err := id(f.Kind, topic, f.Challenge, f.Index)
	if err != nil {
		return nil, err
	}
	if len(f.Soc) < swarm.HashSize || !bytes.Equal(f.Soc[:swarm.HashSize], want) {
		return nil, fmt.Errorf("id not derived from frame: %w", errInvalidSOC)
	}
	if empty(f.Kind) {
		// span 0, no payload
		if len(f.Soc) != swarm.HashSize+swarm.SocSignatureSize+swarm.SpanSize ||
			binary.LittleEndian.Uint64(f.Soc[swarm.HashSize+swarm.SocSignatureSize:]) != 0 {
			return nil, fmt.Errorf("%s chunk not empty: %w", f.Kind, errInvalidSOC)
		}
	}
	return want, nil
}

func verify(f *pb.Broadcast, topic, owner []byte) error {
	want, err := checkFrame(f, topic)
	if err != nil {
		return err
	}
	addr, err := soc.CreateAddress(want, owner)
	if err != nil {
		return err
	}
	if !soc.Valid(swarm.NewChunk(addr, f.Soc)) {
		return fmt.Errorf("not the owner's soc: %w", errInvalidSOC)
	}
	return nil
}

// recoverOwner checks the frame as verify does and returns the owner the
// chunk's signature recovers: the chunk is then valid at keccak256(id | owner).
func recoverOwner(f *pb.Broadcast, topic []byte) ([]byte, error) {
	if _, err := checkFrame(f, topic); err != nil {
		return nil, err
	}
	s, err := soc.FromChunk(swarm.NewChunk(swarm.ZeroAddress, f.Soc))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidSOC, err)
	}
	return s.OwnerAddress(), nil
}

// decodeRoster returns the publisher set a validated ROSTER chunk carries.
func decodeRoster(chunk []byte) ([][]byte, error) {
	off := swarm.HashSize + swarm.SocSignatureSize + swarm.SpanSize
	if len(chunk) < off {
		return nil, errInvalidRoster
	}
	var r pb.Roster
	if err := r.Unmarshal(chunk[off:]); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRoster, err)
	}
	for _, a := range r.Publishers {
		if len(a) != crypto.AddressSize {
			return nil, fmt.Errorf("address length %d: %w", len(a), errInvalidRoster)
		}
	}
	return r.Publishers, nil
}

// RosterPayload returns the payload of a ROSTER chunk naming the publishers.
func RosterPayload(publishers [][]byte) ([]byte, error) {
	return (&pb.Roster{Publishers: publishers}).Marshal()
}
