// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	contractclient "github.com/ethersphere/bee/v2/contracts/client"
	contractserver "github.com/ethersphere/bee/v2/contracts/server"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const (
	directUploadBenchmarkBatchSize   = 16
	directUploadBenchmarkFixtureRing = 1024
)

var (
	directUploadBenchmarkPayloadSizes = []int{1, 32, 128, 512, 1024, 2048, 4096}
	directUploadBenchmarkFileChunks   = []int{1, 16, 128, 1024, 10000}
)

type directUploadBenchmarkMode struct {
	name             string
	chunksPerMessage int
	open             func(context.Context) (storer.PutterSession, error)
}

func BenchmarkDirectUploadProductionPath(b *testing.B) {
	for _, modeName := range []string{"Direct", "Unary", "Streaming", "BatchStreaming16"} {
		b.Run(modeName, func(b *testing.B) {
			for _, payloadSize := range directUploadBenchmarkPayloadSizes {
				b.Run(fmt.Sprintf("payload_%d", payloadSize), func(b *testing.B) {
					for _, fileChunks := range directUploadBenchmarkFileChunks {
						b.Run(fmt.Sprintf("file_chunks_%d", fileChunks), func(b *testing.B) {
							benchmarkDirectUploadProductionPath(b, modeName, payloadSize, fileChunks)
						})
					}
				})
			}
		})
	}
}

func benchmarkDirectUploadProductionPath(b *testing.B, modeName string, payloadSize, fileChunks int) {
	ctx := context.Background()
	backend := newDirectUploadBenchmarkStorer(b)
	mode := newDirectUploadBenchmarkMode(b, modeName, backend.db)
	fixtures := directUploadBenchmarkChunks(b, payloadSize, min(fileChunks, directUploadBenchmarkFixtureRing))
	warmDirectUploadBenchmarkMode(b, ctx, mode, fixtures, fileChunks)
	backend.pushed.Store(0)

	b.ReportAllocs()
	b.SetBytes(int64(payloadSize * fileChunks))

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	b.ResetTimer()
	for range b.N {
		session, err := mode.open(ctx)
		if err != nil {
			b.Fatal(err)
		}
		for i := range fileChunks {
			if err := session.Put(ctx, fixtures[i%len(fixtures)]); err != nil {
				b.Fatal(err)
			}
		}
		if err := session.Done(fixtures[0].Address()); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	runtime.ReadMemStats(&after)

	totalChunks := uint64(b.N) * uint64(fileChunks)
	if got := backend.pushed.Load(); got != totalChunks {
		b.Fatalf("pushed chunks: got %d want %d", got, totalChunks)
	}
	totalBytes := after.TotalAlloc - before.TotalAlloc
	totalAllocs := after.Mallocs - before.Mallocs
	elapsedNS := float64(b.Elapsed().Nanoseconds())
	nsPerChunk := elapsedNS / float64(totalChunks)

	b.ReportMetric(float64(totalBytes)/float64(totalChunks), "B/chunk")
	b.ReportMetric(float64(totalAllocs)/float64(totalChunks), "allocs/chunk")
	if totalAllocs > 0 {
		b.ReportMetric(float64(totalBytes)/float64(totalAllocs), "B/alloc")
	}
	b.ReportMetric(nsPerChunk, "ns/chunk")
	b.ReportMetric(1e9/nsPerChunk, "chunks/s")
	b.ReportMetric(float64(fileChunks), "chunks/file")
	if mode.chunksPerMessage > 0 {
		messageCount := (fileChunks + mode.chunksPerMessage - 1) / mode.chunksPerMessage
		b.ReportMetric(float64(fileChunks)/float64(messageCount), "chunks/message")
	}
}

func warmDirectUploadBenchmarkMode(
	b *testing.B,
	ctx context.Context,
	mode directUploadBenchmarkMode,
	fixtures []swarm.Chunk,
	fileChunks int,
) {
	b.Helper()

	session, err := mode.open(ctx)
	if err != nil {
		b.Fatal(err)
	}
	for i := range fileChunks {
		if err := session.Put(ctx, fixtures[i%len(fixtures)]); err != nil {
			b.Fatal(err)
		}
	}
	if err := session.Done(fixtures[0].Address()); err != nil {
		b.Fatal(err)
	}
}

func newDirectUploadBenchmarkMode(b *testing.B, name string, db *storer.DB) directUploadBenchmarkMode {
	b.Helper()

	if name == "Direct" {
		return directUploadBenchmarkMode{
			name: name,
			open: func(context.Context) (storer.PutterSession, error) {
				return db.DirectUpload(), nil
			},
		}
	}

	client := newDirectUploadBenchmarkGRPCClient(b, db)
	switch name {
	case "Unary":
		return directUploadBenchmarkMode{
			name:             name,
			chunksPerMessage: 1,
			open: func(context.Context) (storer.PutterSession, error) {
				return client.DirectUpload()
			},
		}
	case "Streaming":
		return directUploadBenchmarkMode{
			name:             name,
			chunksPerMessage: 1,
			open:             client.DirectUploadStream,
		}
	case "BatchStreaming16":
		return directUploadBenchmarkMode{
			name:             name,
			chunksPerMessage: directUploadBenchmarkBatchSize,
			open: func(ctx context.Context) (storer.PutterSession, error) {
				return client.DirectUploadBatchStream(ctx, directUploadBenchmarkBatchSize)
			},
		}
	default:
		b.Fatalf("unknown benchmark mode %q", name)
		return directUploadBenchmarkMode{}
	}
}

type directUploadBenchmarkStorer struct {
	db     *storer.DB
	pushed atomic.Uint64
}

func newDirectUploadBenchmarkStorer(b *testing.B) *directUploadBenchmarkStorer {
	b.Helper()

	opts := dbTestOps(swarm.NewAddress(bytes.Repeat([]byte{0x7f}, swarm.HashSize)), 0, nil, nil, time.Second)
	opts.CacheCapacity = 100
	db, err := storer.New(context.Background(), "", opts)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := db.Close(); err != nil {
			b.Error(err)
		}
	})

	backend := &directUploadBenchmarkStorer{db: db}
	quit := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case op := <-db.PusherFeed():
				backend.pushed.Add(1)
				op.Err <- nil
			case <-quit:
				return
			}
		}
	}()
	b.Cleanup(func() {
		close(quit)
		<-done
	})
	return backend
}

func newDirectUploadBenchmarkGRPCClient(b *testing.B, db *storer.DB) *contractclient.Client {
	b.Helper()

	listener := bufconn.Listen(1024 * 1024)
	srv := contractserver.New(contractserver.Config{
		NetStore:          db,
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
	b.Cleanup(func() {
		if err := conn.Close(); err != nil {
			b.Error(err)
		}
		if err := srv.Close(); err != nil {
			b.Error(err)
		}
	})
	return contractclient.NewFromConn(conn, log.Noop)
}

func directUploadBenchmarkChunks(b *testing.B, payloadSize, count int) []swarm.Chunk {
	b.Helper()

	chunks := make([]swarm.Chunk, count)
	for i := range chunks {
		payload := make([]byte, payloadSize)
		for j := range payload {
			payload[j] = byte(i*31 + j)
		}
		ch, err := cac.New(payload)
		if err != nil {
			b.Fatal(err)
		}
		index := make([]byte, postage.IndexSize)
		timestamp := make([]byte, postage.IndexSize)
		binary.BigEndian.PutUint64(index, uint64(i))
		binary.BigEndian.PutUint64(timestamp, uint64(i+1))
		chunks[i] = ch.WithStamp(postage.NewStamp(
			bytes.Repeat([]byte{1}, swarm.HashSize),
			index,
			timestamp,
			bytes.Repeat([]byte{4}, swarm.SocSignatureSize),
		))
	}
	return chunks
}
