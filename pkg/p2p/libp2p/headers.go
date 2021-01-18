// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"context"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/headers/pb"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/swarm"
)

var sendHeadersTimeout = 10 * time.Second

func sendHeaders(ctx context.Context, headers p2p.Headers, stream *stream) error {
	w, r := protobuf.NewWriterAndReader(stream)

	ctx, cancel := context.WithTimeout(ctx, sendHeadersTimeout)
	defer cancel()

	if err := w.WriteMsgWithContext(ctx, headersP2PToPB(headers)); err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	h := new(pb.Headers)
	if err := r.ReadMsgWithContext(ctx, h); err != nil {
		return fmt.Errorf("read message: %w", err)
	}

	stream.headers = headersPBToP2P(h)

	return nil
}

func handleHeaders(headler p2p.HeadlerFunc, stream *stream, peerAddress swarm.Address) error {
	w, r := protobuf.NewWriterAndReader(stream)

	ctx, cancel := context.WithTimeout(context.Background(), sendHeadersTimeout)
	defer cancel()

	headers := new(pb.Headers)
	if err := r.ReadMsgWithContext(ctx, headers); err != nil {
		return fmt.Errorf("read message: %w", err)
	}

	stream.headers = headersPBToP2P(headers)

	var h p2p.Headers
	if headler != nil {
		h = headler(stream.headers, peerAddress)
	}

	if err := w.WriteMsgWithContext(ctx, headersP2PToPB(h)); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	return nil
}

func headersPBToP2P(h *pb.Headers) p2p.Headers {
	p2ph := make(p2p.Headers)
	for _, rh := range h.Headers {
		p2ph[rh.Key] = rh.Value
	}
	return p2ph
}

func headersP2PToPB(h p2p.Headers) *pb.Headers {
	pbh := new(pb.Headers)
	pbh.Headers = make([]*pb.Header, 0)
	for key, value := range h {
		pbh.Headers = append(pbh.Headers, &pb.Header{
			Key:   key,
			Value: value,
		})
	}
	return pbh
}
