// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package recovery_test

import (
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"errors"
	"io/ioutil"
	"testing"
	"time"

	accountingmock "github.com/ethersphere/bee/pkg/accounting/mock"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/netstore"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	pricermock "github.com/ethersphere/bee/pkg/pricer/mock"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/pushsync"
	pushsyncmock "github.com/ethersphere/bee/pkg/pushsync/mock"
	"github.com/ethersphere/bee/pkg/recovery"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	storemock "github.com/ethersphere/bee/pkg/storage/mock"
	chunktesting "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

// TestCallback tests that a  callback can be created and called.
func TestCallback(t *testing.T) {
	// test variables needed to be correctly set for any recovery callback to reach the sender func
	chunkAddr := chunktesting.GenerateTestRandomChunk().Address()
	targets := pss.Targets{[]byte{0xED}}

	//setup the sender
	callbackWasCalled := make(chan bool) // channel to check if callback is called
	pssSender := &mockPssSender{
		callbackC: callbackWasCalled,
	}

	// create recovery callback and call it
	recoveryCallback := recovery.NewCallback(pssSender)
	go recoveryCallback(chunkAddr, targets)

	select {
	case <-callbackWasCalled:
		break
	case <-time.After(100 * time.Millisecond):
		t.Fatal("recovery callback was not called")
	}
}

// CallbackTestCase is a struct used as test cases for the TestCallbackCalls func.
type recoveryCallbackTestCase struct {
	name           string
	ctx            context.Context
	expectsFailure bool
}

// TestCallbackCalls verifies that recovery callbacks are being called as expected when net store attempts to get a chunk.
func TestCallbackCalls(t *testing.T) {
	// generate test chunk, store and publisher
	c := chunktesting.GenerateTestRandomChunk()
	ref := c.Address()
	target := "BE"

	// test cases variables
	targetContext := sctx.SetTargets(context.Background(), target)

	for _, tc := range []recoveryCallbackTestCase{
		{
			name:           "targets set in context",
			ctx:            targetContext,
			expectsFailure: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			callbackWasCalled := make(chan bool, 1) // channel to check if callback is called

			// setup the sender
			pssSender := &mockPssSender{
				callbackC: callbackWasCalled,
			}
			recoverFunc := recovery.NewCallback(pssSender)
			ns := newTestNetStore(t, recoverFunc)

			// fetch test chunk
			_, err := ns.Get(tc.ctx, storage.ModeGetRequest, ref)
			if err != nil && !errors.Is(err, netstore.ErrRecoveryAttempt) && err.Error() != "error decoding prefix string" {
				t.Fatal(err)
			}

			// checks whether the callback is invoked or the test case times out
			select {
			case <-callbackWasCalled:
				if !tc.expectsFailure {
					return
				}
				t.Fatal("recovery callback was unexpectedly called")
			case <-time.After(1000 * time.Millisecond):
				if tc.expectsFailure {
					return
				}
				t.Fatal("recovery callback was not called when expected")
			}
		})
	}
}

// TestNewRepairHandler tests the function of repairing a chunk when a request for chunk repair is received.
func TestNewRepairHandler(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	t.Run("repair-chunk", func(t *testing.T) {
		// generate test chunk, store and publisher
		c1 := chunktesting.GenerateTestRandomChunk()

		// create a mock storer and put a chunk that will be repaired
		mockStorer := storemock.NewStorer()
		defer mockStorer.Close()
		_, err := mockStorer.Put(context.Background(), storage.ModePutRequest, c1)
		if err != nil {
			t.Fatal(err)
		}

		// create a mock pushsync service to push the chunk to its destination
		var receipt *pushsync.Receipt
		pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
			receipt = &pushsync.Receipt{
				Address: swarm.NewAddress(chunk.Address().Bytes()),
			}
			return receipt, nil
		})

		// create the chunk repair handler
		repairHandler := recovery.NewRepairHandler(mockStorer, logger, pushSyncService)

		// invoke the chunk repair handler
		repairHandler(context.Background(), c1.Address().Bytes())

		// check if receipt is received
		if receipt == nil {
			t.Fatal("receipt not received")
		}

		if !receipt.Address.Equal(c1.Address()) {
			t.Fatalf("invalid address in receipt: expected %s received %s", c1.Address(), receipt.Address)
		}

	})

	t.Run("repair-chunk-not-present", func(t *testing.T) {
		// generate test chunk, store and publisher
		c2 := chunktesting.GenerateTestRandomChunk()

		// create a mock storer
		mockStorer := storemock.NewStorer()
		defer mockStorer.Close()

		// create a mock pushsync service
		pushServiceCalled := false
		pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
			pushServiceCalled = true
			return nil, nil
		})

		// create the chunk repair handler
		repairHandler := recovery.NewRepairHandler(mockStorer, logger, pushSyncService)

		// invoke the chunk repair handler
		repairHandler(context.Background(), c2.Address().Bytes())

		if pushServiceCalled {
			t.Fatal("push service called even if the chunk is not present")
		}
	})

	t.Run("repair-chunk-closest-peer-not-present", func(t *testing.T) {
		// generate test chunk, store and publisher
		c3 := chunktesting.GenerateTestRandomChunk()

		// create a mock storer
		mockStorer := storemock.NewStorer()
		defer mockStorer.Close()
		_, err := mockStorer.Put(context.Background(), storage.ModePutRequest, c3)
		if err != nil {
			t.Fatal(err)
		}

		// create a mock pushsync service
		var receiptError error
		pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
			receiptError = errors.New("invalid receipt")
			return nil, receiptError
		})

		// create the chunk repair handler
		repairHandler := recovery.NewRepairHandler(mockStorer, logger, pushSyncService)

		// invoke the chunk repair handler
		repairHandler(context.Background(), c3.Address().Bytes())

		if receiptError == nil {
			t.Fatal("pushsync did not generate a receipt error")
		}
	})
}

// newTestNetStore creates a test store with a set RemoteGet func.
func newTestNetStore(t *testing.T, recoveryFunc recovery.Callback) storage.Storer {
	t.Helper()
	storer := mock.NewStorer()
	logger := logging.New(ioutil.Discard, 5)

	priceHeadlerFunc := func(headers p2p.Headers, addr swarm.Address) p2p.Headers {

		var p []byte
		binary.LittleEndian.PutUint64(p, uint64(10))

		return p2p.Headers{
			"target": []byte("deadbeef"),
			"price":  p,
		}
	}

	mockStorer := storemock.NewStorer()
	serverMockAccounting := accountingmock.NewAccounting()
	pricerMock := pricermock.NewMockService(pricermock.WithPriceHeadlerFunc(priceHeadlerFunc))
	peerID := swarm.MustParseHexAddress("deadbeef")
	ps := mockPeerSuggester{eachPeerRevFunc: func(f topology.EachPeerFunc) error {
		_, _, _ = f(peerID, 0)
		return nil
	}}
	server := retrieval.New(swarm.ZeroAddress, mockStorer, nil, ps, logger, serverMockAccounting, nil, nil)
	recorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
	)
	retrieve := retrieval.New(swarm.ZeroAddress, mockStorer, recorder, ps, logger, serverMockAccounting, pricerMock, nil)
	ns := netstore.New(storer, recoveryFunc, retrieve, logger)
	return ns
}

type mockPeerSuggester struct {
	eachPeerRevFunc func(f topology.EachPeerFunc) error
}

func (s mockPeerSuggester) EachPeer(topology.EachPeerFunc) error {
	return errors.New("not implemented")
}
func (s mockPeerSuggester) EachPeerRev(f topology.EachPeerFunc) error {
	return s.eachPeerRevFunc(f)
}

type mockPssSender struct {
	callbackC chan bool
}

// Send mocks the pss Send function
func (mp *mockPssSender) Send(ctx context.Context, topic pss.Topic, payload []byte, recipient *ecdsa.PublicKey, targets pss.Targets) error {
	mp.callbackC <- true
	return nil
}
