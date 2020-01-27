// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package retrieval_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p/mock"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/retrieval"
	storemock "github.com/ethersphere/bee/pkg/storage/mock"
)

// TestDelivery tests that a naive request -> delivery flow works
func TestDelivery(t *testing.T) {
	logger := logging.New(ioutil.Discard)

	mockStorer := storemock.NewStorer()
	reqAddr := []byte("reqaddr")
	reqData := []byte("data data data")

	// put testdata in the mock store of the server
	mockStorer.Put(context.TODO(), reqAddr, reqData)

	// create the server that will handle the request and will serve the response
	server := retrieval.New(retrieval.Options{
		Storer: mockStorer,
		Logger: logger,
	})
	recorder := mock.NewRecorder(
		mock.WithProtocols(server.Protocol()),
	)

	// client mock storer does not store any data at this point
	// but should be checked at at the end of the test for the
	// presence of the reqAddr key and value to ensure delivery
	// was successful
	clientMockStorer := storemock.NewStorer()

	client := retrieval.New(retrieval.Options{
		Streamer: recorder,
		Storer:   clientMockStorer,
		Logger:   logger,
	})

	peerID := "/p2p/QmZt98UimwpW9ptJumKTq7B7t3FzNfyoWVNGcd8PFCd7XS"
	v, err := client.RetrieveChunk(context.Background(), peerID, reqAddr)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(v, reqData) {
		t.Fatalf("request and response data not equal. got %s want %s", v, reqData)
	}

	records, err := recorder.Records(peerID, "retrieval", "retrieval", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want %v", l, 1)
	}
	record := records[0]

	messages, err := protobuf.ReadMessages(
		bytes.NewReader(record.In()),
		func() protobuf.Message { return new(retrieval.Request) },
	)
	if err != nil {
		t.Fatal(err)
	}

	var reqs []string
	for _, m := range messages {
		reqs = append(reqs, fmt.Sprintf("%x", m.(*retrieval.Request).Addr))
	}

	if len(reqs) != 1 {
		t.Fatalf("got too many requests. want 1 got %d", len(reqs))
	}

	messages, err = protobuf.ReadMessages(
		bytes.NewReader(record.Out()),
		func() protobuf.Message { return new(retrieval.Delivery) },
	)
	if err != nil {
		t.Fatal(err)
	}
	var gotDeliveries []string
	for _, m := range messages {
		gotDeliveries = append(gotDeliveries, string(m.(*retrieval.Delivery).Data))
	}

	if len(gotDeliveries) != 1 {
		t.Fatalf("got too many deliveries. want 1 got %d", len(gotDeliveries))
	}

}
