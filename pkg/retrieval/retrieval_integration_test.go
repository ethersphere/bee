// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package retrieval_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	accountingmock "github.com/ethersphere/bee/v2/pkg/accounting/mock"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/libp2ptest"
	pricermock "github.com/ethersphere/bee/v2/pkg/pricer/mock"
	"github.com/ethersphere/bee/v2/pkg/retrieval"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	topologymock "github.com/ethersphere/bee/v2/pkg/topology/mock"
)

// TestRetrievalIntegration is a regression test for the fix of missing
// FullClose on retrieval streams.
func TestRetrievalIntegration(t *testing.T) {
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

	// Setup chunk
	chunk := testingc.FixtureChunk("0033")

	// Server setup
	serverStorer := &testStorer{ChunkStore: inmemchunkstore.New()}
	err := serverStorer.Put(context.Background(), chunk)
	if err != nil {
		t.Fatal(err)
	}

	serverRetrieval := retrieval.New(
		serverAddr,
		func() (uint8, error) { return swarm.MaxBins, nil },
		serverStorer,
		server,                           // Use libp2p service as streamer
		topologymock.NewTopologyDriver(), // No peers needed for server in this test
		logger.WithValues("retrieval", "server").Build(),
		accountingmock.NewAccounting(),
		pricermock.NewMockService(10, 10),
		nil,
		false,
	)
	t.Cleanup(func() { serverRetrieval.Close() })

	// Register retrieval protocol on server
	if err := server.AddProtocol(serverRetrieval.Protocol()); err != nil {
		t.Fatal(err)
	}

	// Client setup
	clientStorer := &testStorer{ChunkStore: inmemchunkstore.New()}

	// Client needs to know server is the closest peer
	clientTopology := topologymock.NewTopologyDriver(topologymock.WithPeers(serverAddr))

	clientRetrieval := retrieval.New(
		clientAddr,
		func() (uint8, error) { return swarm.MaxBins, nil },
		clientStorer,
		client, // Use libp2p service as streamer
		clientTopology,
		logger.WithValues("retrieval", "client").Build(),
		accountingmock.NewAccounting(),
		pricermock.NewMockService(10, 10),
		nil,
		false,
	)
	t.Cleanup(func() { clientRetrieval.Close() })

	// Register retrieval protocol on client
	if err := client.AddProtocol(clientRetrieval.Protocol()); err != nil {
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

	// Retrieve chunk
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := clientRetrieval.RetrieveChunk(ctx, chunk.Address(), swarm.ZeroAddress)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got.Data(), chunk.Data()) {
		t.Fatalf("request and response data not equal. got %s want %s", got.Data(), chunk.Data())
	}

	// Retrieve non-existing chunk
	nonExistingChunk := testingc.GenerateTestRandomChunk()
	_, err = clientRetrieval.RetrieveChunk(ctx, nonExistingChunk.Address(), swarm.ZeroAddress)
	if !errors.Is(err, topology.ErrNotFound) {
		t.Fatalf("expected error retrieving non-existing chunk, got %v", err)
	}

	// Validate that streams are properly closed by the retrieval service
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
