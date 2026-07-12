// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/ethersphere/bee/v2/contracts/codec"
	"github.com/ethersphere/bee/v2/contracts/pb"
	contractserver "github.com/ethersphere/bee/v2/contracts/server"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/pusher"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

const (
	benchmarkBufferSize = 1 << 20
	benchmarkBatchSize  = 16
)

var (
	benchmarkPayloadSizes = []int{1, 32, 128, 512, 1024, 2048, 4096}
	benchmarkWireSink     []byte
	benchmarkChunkSink    swarm.Chunk
)

type benchmarkFixture struct {
	payloadSize int
	chunk       swarm.Chunk
	message     *pb.ChunkMessage
	wire        []byte
}

func BenchmarkDirectUploadChunkAllocations(b *testing.B) {
	layers := []struct {
		name string
		run  func(*testing.B, benchmarkFixture)
	}{
		{name: "CodecEncode", run: benchmarkCodecEncode},
		{name: "CodecDecode", run: benchmarkCodecDecode},
		{name: "DirectInterface", run: benchmarkDirectInterface},
		{name: "Unary", run: benchmarkUnary},
		{name: "ClientStreaming", run: benchmarkClientStreaming},
		{name: "Batch16", run: benchmarkBatch},
	}

	for _, layer := range layers {
		b.Run(layer.name, func(b *testing.B) {
			for _, payloadSize := range benchmarkPayloadSizes {
				b.Run(fmt.Sprintf("payload_%d", payloadSize), func(b *testing.B) {
					layer.run(b, newBenchmarkFixture(b, payloadSize))
				})
			}
		})
	}
}

func benchmarkDirectInterface(b *testing.B, fixture benchmarkFixture) {
	ctx := context.Background()
	var netStore storer.NetStore = benchmarkNetStore{}
	session := netStore.DirectUpload()
	if err := session.Put(ctx, fixture.chunk); err != nil {
		b.Fatal(err)
	}

	prepareBenchmark(b, fixture)
	for b.Loop() {
		if err := session.Put(ctx, fixture.chunk); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(fixture.payloadSize), "payload_bytes/op")
	b.ReportMetric(float64(len(fixture.chunk.Data())), "chunk_bytes/op")
}

func benchmarkCodecEncode(b *testing.B, fixture benchmarkFixture) {
	encode := func() {
		msg, err := codec.ChunkToProto(fixture.chunk)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkWireSink, err = proto.Marshal(&pb.DirectUploadPutRequest{Chunk: msg})
		if err != nil {
			b.Fatal(err)
		}
	}
	encode()

	prepareBenchmark(b, fixture)
	for b.Loop() {
		encode()
	}
	reportBenchmarkSizes(b, fixture)
}

func benchmarkCodecDecode(b *testing.B, fixture benchmarkFixture) {
	decode := func() {
		req := new(pb.DirectUploadPutRequest)
		if err := proto.Unmarshal(fixture.wire, req); err != nil {
			b.Fatal(err)
		}
		var err error
		benchmarkChunkSink, err = codec.ChunkFromProto(req.GetChunk())
		if err != nil {
			b.Fatal(err)
		}
	}
	decode()

	prepareBenchmark(b, fixture)
	for b.Loop() {
		decode()
	}
	reportBenchmarkSizes(b, fixture)
}

func benchmarkUnary(b *testing.B, fixture benchmarkFixture) {
	ctx := context.Background()
	grpcClient, closeClient := newBenchmarkClient(b)
	defer closeClient()
	sessionID := openBenchmarkSession(b, grpcClient)
	defer cleanupBenchmarkSession(b, grpcClient, sessionID)

	req := &pb.DirectUploadPutRequest{SessionId: sessionID, Chunk: fixture.message}
	if _, err := grpcClient.Put(ctx, req); err != nil {
		b.Fatal(err)
	}

	prepareBenchmark(b, fixture)
	for b.Loop() {
		if _, err := grpcClient.Put(ctx, req); err != nil {
			b.Fatal(err)
		}
	}
	reportBenchmarkSizes(b, fixture)
	b.ReportMetric(1, "chunks/rpc")
}

func benchmarkClientStreaming(b *testing.B, fixture benchmarkFixture) {
	ctx := context.Background()
	grpcClient, closeClient := newBenchmarkClient(b)
	defer closeClient()
	sessionID := openBenchmarkSession(b, grpcClient)
	defer cleanupBenchmarkSession(b, grpcClient, sessionID)

	req := &pb.DirectUploadPutRequest{SessionId: sessionID, Chunk: fixture.message}
	warmup, err := grpcClient.PutStream(ctx)
	if err != nil {
		b.Fatal(err)
	}
	if err := warmup.Send(req); err != nil {
		b.Fatal(err)
	}
	if _, err := warmup.CloseAndRecv(); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(fixture.payloadSize))
	b.ResetTimer()
	stream, err := grpcClient.PutStream(ctx)
	if err != nil {
		b.Fatal(err)
	}
	for range b.N {
		if err := stream.Send(req); err != nil {
			b.Fatal(err)
		}
	}
	if _, err := stream.CloseAndRecv(); err != nil {
		b.Fatal(err)
	}
	b.StopTimer()

	reportBenchmarkSizes(b, fixture)
	b.ReportMetric(float64(b.N), "chunks/stream")
}

func benchmarkBatch(b *testing.B, fixture benchmarkFixture) {
	ctx := context.Background()
	grpcClient, closeClient := newBenchmarkClient(b)
	defer closeClient()
	sessionID := openBenchmarkSession(b, grpcClient)
	defer cleanupBenchmarkSession(b, grpcClient, sessionID)

	batches := make([]*pb.DirectUploadPutBatchRequest, benchmarkBatchSize+1)
	for size := 1; size <= benchmarkBatchSize; size++ {
		chunks := make([]*pb.ChunkMessage, size)
		for i := range chunks {
			chunks[i] = fixture.message
		}
		batches[size] = &pb.DirectUploadPutBatchRequest{SessionId: sessionID, Chunks: chunks}
	}
	if _, err := grpcClient.PutBatch(ctx, batches[benchmarkBatchSize]); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(fixture.payloadSize))
	b.ResetTimer()
	remaining := b.N
	rpcCount := 0
	for remaining > 0 {
		size := min(remaining, benchmarkBatchSize)
		if _, err := grpcClient.PutBatch(ctx, batches[size]); err != nil {
			b.Fatal(err)
		}
		remaining -= size
		rpcCount++
	}
	b.StopTimer()

	reportBenchmarkSizes(b, fixture)
	batchWireSize := proto.Size(batches[benchmarkBatchSize])
	b.ReportMetric(float64(batchWireSize)/benchmarkBatchSize, "wire_bytes/op")
	b.ReportMetric(float64(b.N)/float64(rpcCount), "chunks/rpc")
}

func newBenchmarkFixture(b *testing.B, payloadSize int) benchmarkFixture {
	b.Helper()

	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte(i)
	}
	ch, err := cac.New(payload)
	if err != nil {
		b.Fatal(err)
	}
	ch = ch.WithStamp(postage.NewStamp(
		bytes.Repeat([]byte{1}, swarm.HashSize),
		bytes.Repeat([]byte{2}, postage.IndexSize),
		bytes.Repeat([]byte{3}, postage.IndexSize),
		bytes.Repeat([]byte{4}, 65),
	))
	msg, err := codec.ChunkToProto(ch)
	if err != nil {
		b.Fatal(err)
	}
	wire, err := proto.Marshal(&pb.DirectUploadPutRequest{Chunk: msg})
	if err != nil {
		b.Fatal(err)
	}

	return benchmarkFixture{
		payloadSize: payloadSize,
		chunk:       ch,
		message:     msg,
		wire:        wire,
	}
}

func prepareBenchmark(b *testing.B, fixture benchmarkFixture) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(fixture.payloadSize))
	b.ResetTimer()
}

func reportBenchmarkSizes(b *testing.B, fixture benchmarkFixture) {
	b.Helper()
	b.ReportMetric(float64(fixture.payloadSize), "payload_bytes/op")
	b.ReportMetric(float64(len(fixture.chunk.Data())), "chunk_bytes/op")
	b.ReportMetric(float64(len(fixture.wire)), "wire_bytes/op")
}

func newBenchmarkClient(b *testing.B) (pb.DirectUploadClient, func()) {
	b.Helper()

	listener := bufconn.Listen(benchmarkBufferSize)
	srv := contractserver.New(contractserver.Config{
		NetStore:          benchmarkNetStore{},
		Logger:            log.Noop,
		DisableRPCLogging: true,
	})
	go func() {
		_ = srv.Serve(listener)
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		b.Fatal(err)
	}
	grpcClient := pb.NewDirectUploadClient(conn)

	return grpcClient, func() {
		if err := conn.Close(); err != nil {
			b.Error(err)
		}
		if err := srv.Close(); err != nil {
			b.Error(err)
		}
	}
}

func openBenchmarkSession(b *testing.B, grpcClient pb.DirectUploadClient) uint64 {
	b.Helper()
	resp, err := grpcClient.Open(context.Background(), &pb.OpenDirectUploadRequest{})
	if err != nil {
		b.Fatal(err)
	}
	return resp.GetSessionId()
}

func cleanupBenchmarkSession(b *testing.B, grpcClient pb.DirectUploadClient, sessionID uint64) {
	b.Helper()
	if _, err := grpcClient.Cleanup(context.Background(), &pb.DirectUploadCleanupRequest{SessionId: sessionID}); err != nil {
		b.Error(err)
	}
}

type benchmarkNetStore struct{}

func (benchmarkNetStore) DirectUpload() storer.PutterSession {
	return benchmarkPutterSession{}
}

func (benchmarkNetStore) Download(bool) storage.Getter {
	return storage.GetterFunc(func(context.Context, swarm.Address) (swarm.Chunk, error) {
		return nil, storage.ErrNotFound
	})
}

func (benchmarkNetStore) PusherFeed() <-chan *pusher.Op {
	return nil
}

type benchmarkPutterSession struct{}

func (benchmarkPutterSession) Put(context.Context, swarm.Chunk) error {
	return nil
}

func (benchmarkPutterSession) Done(swarm.Address) error {
	return nil
}

func (benchmarkPutterSession) Cleanup() error {
	return nil
}
