// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package retrieval_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"io/ioutil"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/retrieval"
	pb "github.com/ethersphere/bee/pkg/retrieval/pb"
	storemock "github.com/ethersphere/bee/pkg/storage/mock"
)

var testTimeout = 5 * time.Second

// TestDelivery tests that a naive request -> delivery flow works.
func TestDelivery(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	mockStorer := storemock.NewStorer()
	reqAddr := []byte("reqaddr")
	reqData := []byte("data data data")

	// put testdata in the mock store of the server
	_ = mockStorer.Put(context.TODO(), reqAddr, reqData)

	// create the server that will handle the request and will serve the response
	server := retrieval.New(retrieval.Options{
		Storer: mockStorer,
		Logger: logger,
	})
	recorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
	)

	// client mock storer does not store any data at this point
	// but should be checked at at the end of the test for the
	// presence of the reqAddr key and value to ensure delivery
	// was successful
	clientMockStorer := storemock.NewStorer()

	ps := mockPeerSuggester{spFunc: func(_ []byte) (string, error) { return "testpeeraddr", nil }}
	client := retrieval.New(retrieval.Options{
		Streamer:      recorder,
		PeerSuggester: ps,
		Storer:        clientMockStorer,
		Logger:        logger,
	})
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	go func() {
		v, err := client.RetrieveChunk(ctx, reqAddr)
		if err != nil {
			return
		}
		if !bytes.Equal(v, reqData) {
			t.Fatalf("request and response data not equal. got %s want %s", v, reqData)
		}
	}()
	time.Sleep(1 * time.Second)

	peerID, _ := ps.SuggestPeer(nil)
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
		func() protobuf.Message { return new(pb.Request) },
	)
	if err != nil {
		t.Fatal(err)
	}
	var reqs []string
	for _, m := range messages {
		reqs = append(reqs, hex.EncodeToString(m.(*pb.Request).Addr))
	}

	if len(reqs) != 1 {
		t.Fatalf("got too many requests. want 1 got %d", len(reqs))
	}

	messages, err = protobuf.ReadMessages(
		bytes.NewReader(record.Out()),
		func() protobuf.Message { return new(pb.Delivery) },
	)
	if err != nil {
		t.Fatal(err)
	}
	var gotDeliveries []string
	for _, m := range messages {
		gotDeliveries = append(gotDeliveries, string(m.(*pb.Delivery).Data))
	}

	if len(gotDeliveries) != 1 {
		t.Fatalf("got too many deliveries. want 1 got %d", len(gotDeliveries))
	}

}

type mockPeerSuggester struct {
	spFunc func([]byte) (string, error)
}

func (v mockPeerSuggester) SuggestPeer(addr []byte) (string, error) {
	return v.spFunc(addr)
}
