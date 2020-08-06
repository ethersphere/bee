// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package recovery_test

import (
	"context"
	"errors"
	"io/ioutil"
	"testing"
	"time"

	accountingmock "github.com/ethersphere/bee/pkg/accounting/mock"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/netstore"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
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
	"github.com/ethersphere/bee/pkg/trojan"
)

// TestRecoveryHook tests that a recovery hook can be created and called.
func TestRecoveryHook(t *testing.T) {
	// test variables needed to be correctly set for any recovery hook to reach the sender func
	chunkAddr := chunktesting.GenerateTestRandomChunk().Address()
	target := "0xED"
	ctx := sctx.SetTargets(context.Background(), target)
	logger := logging.New(ioutil.Discard, 0)

	// setup the sender
	hookWasCalled := false // test variable to check if hook is called
	testSender := func(ctx context.Context, targets trojan.Targets, topic trojan.Topic, payload []byte) (*pss.Monitor, error) {
		hookWasCalled = true
		return nil, nil
	}

	// create recovery hook and call it
	recoveryHook := recovery.NewRecoveryHook(testSender, logger)
	if err := recoveryHook(ctx, chunkAddr); err != nil {
		t.Fatal(err)
	}

	if hookWasCalled != true {
		t.Fatalf("recovery hook was not called")
	}
}

// RecoveryHookTestCase is a struct used as test cases for the TestRecoveryHookCalls func.
type recoveryHookTestCase struct {
	name           string
	ctx            context.Context
	expectsFailure bool
}

// TestRecoveryHookCalls verifies that recovery hooks are being called as expected when net store attempts to get a chunk.
func TestRecoveryHookCalls(t *testing.T) {
	// generate test chunk, store and publisher
	c := chunktesting.GenerateTestRandomChunk()
	ref := c.Address()
	target := "0xBE"
	logger := logging.New(ioutil.Discard, 0)

	// test cases variables
	dummyContext := context.Background() // has no publisher
	targetContext := sctx.SetTargets(context.Background(), target)

	for _, tc := range []recoveryHookTestCase{
		{
			name:           "no targets in context",
			ctx:            dummyContext,
			expectsFailure: true,
		},
		{
			name:           "targets set in context",
			ctx:            targetContext,
			expectsFailure: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hookWasCalled := make(chan bool, 1) // channel to check if hook is called

			// setup recovery hook
			testHook := func(ctx context.Context, targets trojan.Targets, topic trojan.Topic, payload []byte) (*pss.Monitor, error) {
				hookWasCalled <- true
				return nil, nil
			}
			recoverFunc := recovery.NewRecoveryHook(testHook, logger)
			ns := newTestNetStore(t, recoverFunc)

			// fetch test chunk
			_, err := ns.Get(tc.ctx, storage.ModeGetRequest, ref)
			if err != nil && !errors.Is(err, netstore.ErrRecoveryAttempt) && err.Error() != "netstore retrieve chunk: get closest: no peer found" {
				t.Fatal(err)
			}

			// checks whether the callback is invoked or the test case times out
			select {
			case <-hookWasCalled:
				if !tc.expectsFailure {
					return
				}
				t.Fatal("recovery hook was unexpectedly called")
			case <-time.After(100 * time.Millisecond):
				if tc.expectsFailure {
					return
				}
				t.Fatal("recovery hook was not called when expected")
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

		//create a trojan message to trigger the repair of the chunk
		testTopic := trojan.NewTopic("foo")
		maxPayload := make([]byte, swarm.SectionSize)
		var msg trojan.Message
		copy(maxPayload, c1.Address().Bytes())
		if msg, err = trojan.NewMessage(testTopic, maxPayload); err != nil {
			t.Fatal(err)
		}

		// invoke the chunk repair handler
		repairHandler(msg)

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

		//create a trojan message to trigger the repair of the chunk
		testTopic := trojan.NewTopic("foo")
		maxPayload := make([]byte, swarm.SectionSize)
		var msg trojan.Message
		copy(maxPayload, c2.Address().Bytes())
		msg, err := trojan.NewMessage(testTopic, maxPayload)
		if err != nil {
			t.Fatal(err)
		}

		// invoke the chunk repair handler
		repairHandler(msg)

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

		//create a trojan message to trigger the repair of the chunk
		testTopic := trojan.NewTopic("foo")
		maxPayload := make([]byte, swarm.SectionSize)
		var msg trojan.Message
		copy(maxPayload, c3.Address().Bytes())
		msg, err = trojan.NewMessage(testTopic, maxPayload)
		if err != nil {
			t.Fatal(err)
		}

		// invoke the chunk repair handler
		repairHandler(msg)

		if receiptError == nil {
			t.Fatal("pushsync did not generate a receipt error")
		}
	})
}

// newTestNetStore creates a test store with a set RemoteGet func.
func newTestNetStore(t *testing.T, recoveryFunc recovery.RecoveryHook) storage.Storer {
	t.Helper()
	storer := mock.NewStorer()
	logger := logging.New(ioutil.Discard, 5)

	mockStorer := storemock.NewStorer()
	serverMockAccounting := accountingmock.NewAccounting()
	price := uint64(12345)
	pricerMock := accountingmock.NewPricer(price, price)
	peerID := swarm.MustParseHexAddress("deadbeef")
	ps := mockPeerSuggester{eachPeerRevFunc: func(f topology.EachPeerFunc) error {
		_, _, _ = f(peerID, 0)
		return nil
	}}
	server := retrieval.New(retrieval.Options{
		Storer:     mockStorer,
		Logger:     logger,
		Accounting: serverMockAccounting,
	})
	recorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
	)
	retrieve := retrieval.New(retrieval.Options{
		Streamer:    recorder,
		ChunkPeerer: ps,
		Storer:      mockStorer,
		Logger:      logger,
		Accounting:  serverMockAccounting,
		Pricer:      pricerMock,
	})

	ns := netstore.New(storer, recoveryFunc, nil, retrieve, logger, nil)
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
