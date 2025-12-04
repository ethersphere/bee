// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	accountingmock "github.com/ethersphere/bee/v2/pkg/accounting/mock"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/libp2ptest"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	pricermock "github.com/ethersphere/bee/v2/pkg/pricer/mock"
	"github.com/ethersphere/bee/v2/pkg/pushsync"
	"github.com/ethersphere/bee/v2/pkg/soc"
	stabilizationmock "github.com/ethersphere/bee/v2/pkg/stabilization/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	topologymock "github.com/ethersphere/bee/v2/pkg/topology/mock"
)

// TestPushSyncIntegration is a regression test for the fix of missing
// FullClose on pushsync streams.
func TestPushSyncIntegration(t *testing.T) {
	// Capture logs to check for errors
	var logMessages libp2ptest.SafeBuffer

	// Enable debug logging
	logger := log.NewLogger("test",
		log.WithVerbosity(log.VerbosityDebug),
		log.WithJSONOutput(),
		log.WithSink(&logMessages),
	)

	// Create two libp2p services
	server, serverAddr := libp2ptest.NewLibp2pService(t, 1, logger.WithValues("libp2p", "server").Build())
	client, clientAddr := libp2ptest.NewLibp2pService(t, 1, logger.WithValues("libp2p", "client").Build())

	// Server setup
	serverStorer := &testIntegrationStorer{ChunkStore: inmemchunkstore.New()}
	serverSigner := crypto.NewDefaultSigner(mustGenerateKey(t))

	serverPushSync := pushsync.New(
		serverAddr,
		1,
		common.HexToHash("0x1").Bytes(),
		server, // Use libp2p service as streamer
		serverStorer,
		func() (uint8, error) { return 0, nil },
		topologymock.NewTopologyDriver(), // No peers needed for server in this test
		true,
		func(swarm.Chunk) {}, // unwrap
		func(*soc.SOC) {},    // gsocHandler
		func(c swarm.Chunk) (swarm.Chunk, error) {
			return c.WithStamp(postagetesting.MustNewValidStamp(serverSigner, c.Address())), nil
		},
		logger.WithValues("pushsync", "server").Build(),
		accountingmock.NewAccounting(),
		pricermock.NewMockService(10, 10),
		serverSigner,
		nil,
		stabilizationmock.NewSubscriber(true),
		0,
	)
	t.Cleanup(func() { serverPushSync.Close() })

	// Register pushsync protocol on server
	if err := server.AddProtocol(serverPushSync.Protocol()); err != nil {
		t.Fatal(err)
	}

	// Client setup
	clientStorer := &testIntegrationStorer{ChunkStore: inmemchunkstore.New()}
	clientSigner := crypto.NewDefaultSigner(mustGenerateKey(t))

	// Client needs to know server is the closest peer
	clientTopology := topologymock.NewTopologyDriver(topologymock.WithPeers(serverAddr))

	clientPushSync := pushsync.New(
		clientAddr,
		1,
		common.HexToHash("0x1").Bytes(),
		client, // Use libp2p service as streamer
		clientStorer,
		func() (uint8, error) { return 0, nil },
		clientTopology,
		true,
		func(swarm.Chunk) {}, // unwrap
		func(*soc.SOC) {},    // gsocHandler
		func(c swarm.Chunk) (swarm.Chunk, error) {
			return c.WithStamp(postagetesting.MustNewValidStamp(clientSigner, c.Address())), nil
		},
		logger.WithValues("pushsync", "client").Build(),
		accountingmock.NewAccounting(),
		pricermock.NewMockService(10, 10),
		clientSigner,
		nil,
		stabilizationmock.NewSubscriber(true),
		0,
	)
	t.Cleanup(func() { clientPushSync.Close() })

	// Register pushsync protocol on client
	if err := client.AddProtocol(clientPushSync.Protocol()); err != nil {
		t.Fatal(err)
	}

	// Connect client to server
	serverAddrs, err := server.Addresses()
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Connect(context.Background(), serverAddrs)
	if err != nil {
		t.Fatal(err)
	}

	// Push chunk
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Generate a chunk that is closer to the server than the client
	var chunk swarm.Chunk
	for {
		chunk = testingc.GenerateTestRandomChunk()
		// Check if chunk is closer to server
		if swarm.Proximity(serverAddr.Bytes(), chunk.Address().Bytes()) > swarm.Proximity(clientAddr.Bytes(), chunk.Address().Bytes()) {
			break
		}
	}

	_, err = clientPushSync.PushChunkToClosest(ctx, chunk)
	if err != nil {
		t.Fatal(err)
	}

	// Verify chunk is stored on server
	has, err := serverStorer.Has(ctx, chunk.Address())
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("chunk not found on server")
	}

	// Validate that streams are properly closed by the pushsync service
	// by checking for "stream reset" log entries
	streamResetLogLine := ""
	for line := range strings.SplitSeq(logMessages.String(), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("failed to unmarshal log entry: %v", err)
		}
		errorString, ok := entry["error"].(string)
		if !ok {
			continue
		}
		if strings.Contains(errorString, "stream reset") {
			streamResetLogLine = line
			break
		}
	}
	if streamResetLogLine != "" {
		t.Errorf("found stream reset log line: %s", streamResetLogLine)
	}
}

type testIntegrationStorer struct {
	storage.ChunkStore
}

func (t *testIntegrationStorer) Report(context.Context, swarm.Chunk, storage.ChunkState) error {
	return nil
}

func (t *testIntegrationStorer) ReservePutter() storage.Putter {
	return t.ChunkStore
}

func mustGenerateKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	return k
}
