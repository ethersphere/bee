// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api/pb"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// chunkStreamFuzzChunks returns a valid CAC and a valid SOC wrapping it, built
// the way a client would, to seed the chunk stream fuzz targets.
func chunkStreamFuzzChunks(tb testing.TB) (swarm.Chunk, swarm.Chunk) {
	tb.Helper()

	ch, err := cac.New([]byte("chunk stream fuzz seed"))
	if err != nil {
		tb.Fatal(err)
	}
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		tb.Fatal(err)
	}
	sch, err := soc.New(make([]byte, swarm.HashSize), ch).Sign(crypto.NewDefaultSigner(privKey))
	if err != nil {
		tb.Fatal(err)
	}
	return ch, sch
}

func marshalStreamRequest(tb testing.TB, req *pb.Request) []byte {
	tb.Helper()

	data, err := req.Marshal()
	if err != nil {
		tb.Fatal(err)
	}
	return data
}

// FuzzChunkStreamRequest drives arbitrary websocket messages through the
// decoding the bidirectional /chunks/stream read loop applies to every client
// message. A message that decodes must survive a marshal round trip unchanged,
// and the chunk of an upload must only be accepted when it is a valid CAC or
// SOC with the requested address.
func FuzzChunkStreamRequest(f *testing.F) {
	ch, sch := chunkStreamFuzzChunks(f)

	f.Add(marshalStreamRequest(f, &pb.Request{Body: &pb.Request_Get{Get: &pb.GetRequest{
		Address: ch.Address().Bytes(),
		Cache:   pb.CacheOption_CACHE_DISABLE,
	}}}))
	f.Add(marshalStreamRequest(f, &pb.Request{Body: &pb.Request_Put{Put: &pb.PutRequest{
		Address: ch.Address().Bytes(),
		Data:    ch.Data(),
		BatchId: make([]byte, swarm.HashSize),
	}}}))
	f.Add(marshalStreamRequest(f, &pb.Request{Body: &pb.Request_Put{Put: &pb.PutRequest{
		Address: sch.Address().Bytes(),
		Data:    sch.Data(),
		Stamp:   make([]byte, 113),
	}}}))
	// Address that does not match the data.
	f.Add(marshalStreamRequest(f, &pb.Request{Body: &pb.Request_Put{Put: &pb.PutRequest{
		Address: sch.Address().Bytes(),
		Data:    ch.Data(),
	}}}))
	// Data one byte over the SOC size limit.
	f.Add(marshalStreamRequest(f, &pb.Request{Body: &pb.Request_Put{Put: &pb.PutRequest{
		Address: ch.Address().Bytes(),
		Data:    make([]byte, swarm.SocMaxChunkSize+1),
	}}}))
	putMsg := marshalStreamRequest(f, &pb.Request{Body: &pb.Request_Put{Put: &pb.PutRequest{
		Address: ch.Address().Bytes(),
		Data:    ch.Data(),
	}}})
	f.Add(putMsg[:len(putMsg)/2])
	f.Add([]byte{})
	f.Add([]byte{0x0a, 0x00}) // empty Get body
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01})

	f.Fuzz(func(t *testing.T, msg []byte) {
		if len(msg) > maxStreamFrameSize {
			return
		}

		req := &pb.Request{}
		if err := req.Unmarshal(msg); err != nil {
			return
		}

		data, err := req.Marshal()
		if err != nil {
			t.Fatalf("marshal decoded request: %v", err)
		}
		again := &pb.Request{}
		if err := again.Unmarshal(data); err != nil {
			t.Fatalf("unmarshal re-marshaled request: %v", err)
		}
		dataAgain, err := again.Marshal()
		if err != nil {
			t.Fatalf("marshal round-tripped request: %v", err)
		}
		if !bytes.Equal(dataAgain, data) {
			t.Fatalf("round trip changed the request: %v != %v", again, req)
		}

		put := req.GetPut()
		if put == nil || len(put.Address) != swarm.HashSize || len(put.Data) < swarm.SpanSize {
			return
		}
		checkChunkForAddress(t, put.Address, put.Data)
	})
}

// FuzzChunkForAddress drives arbitrary address and data pairs through
// chunkForAddress, which decides whether an uploaded chunk is a CAC or a SOC.
func FuzzChunkForAddress(f *testing.F) {
	ch, sch := chunkStreamFuzzChunks(f)

	f.Add(ch.Address().Bytes(), ch.Data())
	f.Add(sch.Address().Bytes(), sch.Data())
	f.Add(ch.Address().Bytes(), sch.Data())
	f.Add(sch.Address().Bytes(), ch.Data())
	f.Add(sch.Address().Bytes(), sch.Data()[:len(sch.Data())-1])
	f.Add(make([]byte, swarm.HashSize), make([]byte, swarm.SpanSize))
	f.Add(make([]byte, swarm.HashSize), make([]byte, swarm.ChunkWithSpanSize+1))
	f.Add([]byte{}, []byte{})

	f.Fuzz(func(t *testing.T, address, data []byte) {
		if len(address) != swarm.HashSize || len(data) < swarm.SpanSize || len(data) > maxStreamFrameSize {
			return
		}
		checkChunkForAddress(t, address, data)
	})
}

func checkChunkForAddress(t *testing.T, address, data []byte) {
	t.Helper()

	got, ok := chunkForAddress(address, data)
	if !ok {
		return
	}
	if !got.Address().Equal(swarm.NewAddress(address)) {
		t.Fatalf("accepted chunk has address %s, requested %x", got.Address(), address)
	}
	if !bytes.Equal(got.Data(), data) {
		t.Fatal("accepted chunk data differs from the uploaded data")
	}
	if !cac.Valid(got) && !soc.Valid(got) {
		t.Fatalf("accepted chunk %s is neither a valid CAC nor a valid SOC", got.Address())
	}
}
