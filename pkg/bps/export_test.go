// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps

import "github.com/ethersphere/bee/v2/pkg/bps/pb"

const (
	ProtocolName    = protocolName
	ProtocolVersion = protocolVersion
	StreamName      = streamName
)

var (
	ErrNotBroker       = errNotBroker
	ErrJoinRefused     = errJoinRefused
	ErrInvalidJoin     = errInvalidJoin
	ErrSessionClosed   = errSessionClosed
	ErrProtocolError   = errProtocolError
	ErrInvalidDelivery = errInvalidDelivery
	ErrChannelEnded    = errChannelEnded
)

// Verify checks a chunk against the frame fields it was derived from and its owner.
func Verify(kind Kind, challenge []byte, index uint64, chunk, topic, owner []byte) error {
	_, err := verify(&pb.Broadcast{Soc: chunk, Kind: kind.proto(), Challenge: challenge, Index: index}, topic, owner)
	return err
}

// EncodeRosterMsg returns the payload of a ROSTER chunk naming the publishers.
func EncodeRosterMsg(publishers [][]byte) ([]byte, error) {
	return (&pb.Roster{Publishers: publishers}).Marshal()
}
