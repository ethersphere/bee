// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"fmt"

	"github.com/ethersphere/bee/v2/contracts/pb"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func ChunkToProto(ch swarm.Chunk) (*pb.ChunkMessage, error) {
	if ch == nil {
		return nil, fmt.Errorf("codec: nil chunk")
	}
	msg := &pb.ChunkMessage{
		Address: ch.Address().Bytes(),
		Data:    ch.Data(),
	}
	if st := ch.Stamp(); st != nil {
		b, err := st.MarshalBinary()
		if err != nil {
			return nil, err
		}
		msg.Stamp = b
	}
	return msg, nil
}

func ChunkFromProto(msg *pb.ChunkMessage) (swarm.Chunk, error) {
	if msg == nil {
		return nil, fmt.Errorf("codec: nil chunk message")
	}
	ch := swarm.NewChunk(swarm.NewAddress(msg.Address), msg.Data)
	if len(msg.Stamp) == 0 {
		return ch, nil
	}
	st := new(postage.Stamp)
	if err := st.UnmarshalBinary(msg.Stamp); err != nil {
		return nil, err
	}
	return ch.WithStamp(st), nil
}
