// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pricer_test

import (
	"encoding/binary"
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/headerutils"
	mockkad "github.com/ethersphere/bee/pkg/kademlia/mock"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/pricer"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestPriceTableDefaultTables(t *testing.T) {

	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()

	overlay := swarm.MustParseHexAddress("e5ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700")

	defer store.Close()
	pricer := pricer.New(logger, store, overlay, 10)

	kad := mockkad.NewMockKademlia(mockkad.WithDepthCalls(0, 0, 2, 3, 5, 9))
	pricer.SetKademlia(kad)

	table0 := []uint64{10}
	table1 := []uint64{30, 20, 10}
	table2 := []uint64{40, 30, 20, 10}
	table3 := []uint64{60, 50, 40, 30, 20, 10}
	table4 := []uint64{100, 90, 80, 70, 60, 50, 40, 30, 20, 10}

	getTable0 := pricer.PriceTable()
	if !reflect.DeepEqual(table0, getTable0) {
		t.Fatalf("returned table does not match, got %+v expected %+v", getTable0, table0)
	}

	getTable1 := pricer.PriceTable()
	if !reflect.DeepEqual(table1, getTable1) {
		t.Fatalf("returned table does not match, got %+v expected %+v", getTable1, table1)
	}

	getTable2 := pricer.PriceTable()
	if !reflect.DeepEqual(table2, getTable2) {
		t.Fatalf("returned table does not match, got %+v expected %+v", getTable2, table2)
	}

	getTable3 := pricer.PriceTable()
	if !reflect.DeepEqual(table3, getTable3) {
		t.Fatalf("returned table does not match, got %+v expected %+v", getTable3, table3)
	}

	getTable4 := pricer.PriceTable()
	if !reflect.DeepEqual(table4, getTable4) {
		t.Fatalf("returned table does not match, got %+v expected %+v", getTable4, table4)
	}

}

func TestPeerPrice(t *testing.T) {

	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	overlay := swarm.MustParseHexAddress("0000617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700")

	peer := swarm.MustParseHexAddress("e5ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700")
	chunksByPOToPeer := []swarm.Address{
		swarm.MustParseHexAddress("05ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700"),
		swarm.MustParseHexAddress("95ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700"),
		swarm.MustParseHexAddress("c5ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700"),
		swarm.MustParseHexAddress("f0ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700"),
		swarm.MustParseHexAddress("e6ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700"),
	}

	pricer := pricer.New(logger, store, overlay, 10)

	kad := mockkad.NewMockKademlia(mockkad.WithDepth(3))
	pricer.SetKademlia(kad)

	peerTable := []uint64{55, 45, 35, 25, 15}
	err := pricer.NotifyPriceTable(peer, peerTable)
	if err != nil {
		t.Fatal(err)
	}

	for i, ch := range chunksByPOToPeer {

		getPrice := pricer.PeerPrice(peer, ch)
		if getPrice != peerTable[i] {
			t.Fatalf("unexpected PeerPrice, got %v expected %v", getPrice, peerTable[i])
		}
	}

	neighborhoodPeer := swarm.MustParseHexAddress("00ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700")
	neighborhoodPeerTable := []uint64{36, 26, 16, 6}

	err = pricer.NotifyPriceTable(neighborhoodPeer, neighborhoodPeerTable)
	if err != nil {
		t.Fatal(err)
	}

	chunksByPOToNeighborhoodPeer := []swarm.Address{
		swarm.MustParseHexAddress("95ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700"),
		swarm.MustParseHexAddress("55ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700"),
		swarm.MustParseHexAddress("35ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700"),
		swarm.MustParseHexAddress("10ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700"),
	}

	neighborhoodPeerExpectedPrices := []uint64{36, 26, 16, 0}

	for i, ch := range chunksByPOToNeighborhoodPeer {
		getPrice := pricer.PeerPrice(neighborhoodPeer, ch)
		if getPrice != neighborhoodPeerExpectedPrices[i] {
			t.Fatalf("unexpected PeerPrice, got %v expected %v", getPrice, neighborhoodPeerExpectedPrices[i])
		}
	}

}

func TestPriceForPeer(t *testing.T) {

	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	peer := swarm.MustParseHexAddress("0000617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700")

	overlay := swarm.MustParseHexAddress("e5ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700")

	chunksByPOToOverlay := []swarm.Address{
		swarm.MustParseHexAddress("05ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700"),
		swarm.MustParseHexAddress("95ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700"),
		swarm.MustParseHexAddress("c5ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700"),
		swarm.MustParseHexAddress("f0ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700"),
		swarm.MustParseHexAddress("e6ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700"),
	}

	pricer := pricer.New(logger, store, overlay, 10)

	kad := mockkad.NewMockKademlia(mockkad.WithDepth(4))
	pricer.SetKademlia(kad)

	defaultTable := []uint64{50, 40, 30, 20, 10}

	for i, ch := range chunksByPOToOverlay {

		getPrice := pricer.PriceForPeer(peer, ch)
		if getPrice != defaultTable[i] {
			t.Fatalf("unexpected PeerPrice, got %v expected %v", getPrice, defaultTable[i])
		}
	}

	neighborhoodPeer := swarm.MustParseHexAddress("e6ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700")
	expectedPricesInNeighborhood := []uint64{50, 40, 30, 20, 0}

	for i, ch := range chunksByPOToOverlay {
		getPrice := pricer.PeerPrice(neighborhoodPeer, ch)
		if getPrice != expectedPricesInNeighborhood[i] {
			t.Fatalf("unexpected PeerPrice, got %v expected %v", getPrice, expectedPricesInNeighborhood[i])
		}
	}

}

func TestNotifyPriceTable(t *testing.T) {

	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()

	overlay := swarm.MustParseHexAddress("e5ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700")

	defer store.Close()
	pricer := pricer.New(logger, store, overlay, 10)

	kad := mockkad.NewMockKademlia(mockkad.WithDepth(0))
	pricer.SetKademlia(kad)

	peer := swarm.MustParseHexAddress("ffff617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700")
	peerTable := []uint64{66, 55, 44, 33, 22, 11}

	err := pricer.NotifyPriceTable(peer, peerTable)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < len(peerTable); i++ {
		getPrice, err := pricer.PeerPricePO(peer, uint8(i))
		if err != nil {
			t.Fatal(err)
		}
		if getPrice != peerTable[i] {
			t.Fatalf("unexpected PeerPricePO, got %v expected %v", getPrice, peerTable[i])
		}
	}

}

func TestNotifyPeerPrice(t *testing.T) {

	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()

	overlay := swarm.MustParseHexAddress("e5ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700")

	defer store.Close()
	pricer := pricer.New(logger, store, overlay, 10)

	kad := mockkad.NewMockKademlia(mockkad.WithDepth(0))
	pricer.SetKademlia(kad)

	peer := swarm.MustParseHexAddress("ffff617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700")
	peerTable := []uint64{33, 22, 11}

	err := pricer.NotifyPriceTable(peer, peerTable)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < len(peerTable); i++ {
		getPrice, err := pricer.PeerPricePO(peer, uint8(i))
		if err != nil {
			t.Fatal(err)
		}
		if getPrice != peerTable[i] {
			t.Fatalf("unexpected PeerPricePO, got %v expected %v", getPrice, peerTable[i])
		}
	}

	err = pricer.NotifyPeerPrice(peer, 48, 8)
	if err != nil {
		t.Fatal(err)
	}

	expectedTable := []uint64{33, 22, 11, 98, 88, 78, 68, 58, 48}

	for i := 0; i < len(expectedTable); i++ {
		getPrice, err := pricer.PeerPricePO(peer, uint8(i))
		if err != nil {
			t.Fatal(err)
		}
		if getPrice != expectedTable[i] {
			t.Fatalf("unexpected PeerPricePO, got %v expected %v", getPrice, expectedTable[i])
		}
	}

	err = pricer.NotifyPeerPrice(peer, 43, 9)
	if err != nil {
		t.Fatal(err)
	}

	expectedTable = []uint64{33, 22, 11, 98, 88, 78, 68, 58, 48, 43}

	for i := 0; i < len(expectedTable); i++ {
		getPrice, err := pricer.PeerPricePO(peer, uint8(i))
		if err != nil {
			t.Fatal(err)
		}
		if getPrice != expectedTable[i] {
			t.Fatalf("unexpected PeerPricePO, got %v expected %v", getPrice, expectedTable[i])
		}
	}

	err = pricer.NotifyPeerPrice(peer, 60, 5)
	if err != nil {
		t.Fatal(err)
	}

	expectedTable = []uint64{33, 22, 11, 98, 88, 60, 68, 58, 48, 43}

	for i := 0; i < len(expectedTable); i++ {
		getPrice, err := pricer.PeerPricePO(peer, uint8(i))
		if err != nil {
			t.Fatal(err)
		}
		if getPrice != expectedTable[i] {
			t.Fatalf("unexpected PeerPricePO, got %v expected %v", getPrice, expectedTable[i])
		}
	}
}

func TestPricerHeadler(t *testing.T) {

	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()

	overlay := swarm.MustParseHexAddress("e5ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700")

	defer store.Close()
	pricer := pricer.New(logger, store, overlay, 10)

	kad := mockkad.NewMockKademlia(mockkad.WithDepth(2))
	pricer.SetKademlia(kad)

	peer := swarm.MustParseHexAddress("07e0d4ba628ad700fff48b16b52953487a22589362e34efdd7e0d4ba628ad700")

	chunkAddress := swarm.MustParseHexAddress("7a22589362e34efdd7e0d4ba628ad7007a22589362e34efdd7e0d4ba628ad700")
	requestHeaders, err := headerutils.MakePricingHeaders(50, chunkAddress)
	if err != nil {
		t.Fatal(err)
	}
	responseHeaders := pricer.PriceHeadler(requestHeaders, peer)

	if !reflect.DeepEqual(requestHeaders["target"], responseHeaders["target"]) {
		t.Fatalf("targets don't match, got %v, want %v", responseHeaders["target"], requestHeaders["target"])
	}

	chunkPriceInRequest := make([]byte, 8)
	binary.LittleEndian.PutUint64(chunkPriceInRequest, uint64(50))

	chunkPriceInResponse := make([]byte, 8)
	binary.LittleEndian.PutUint64(chunkPriceInResponse, uint64(30))

	if !reflect.DeepEqual(requestHeaders["price"], chunkPriceInRequest) {
		t.Fatalf("targets don't match, got %v, want %v", responseHeaders["price"], chunkPriceInRequest)
	}

	if !reflect.DeepEqual(responseHeaders["price"], chunkPriceInResponse) {
		t.Fatalf("targets don't match, got %v, want %v", responseHeaders["price"], chunkPriceInResponse)
	}

}

func TestPricerHeadlerBadHeaders(t *testing.T) {

	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()

	overlay := swarm.MustParseHexAddress("e5ef617cadab2af7fff48b16b52953487a22589362e34efdd7e0d4ba628ad700")

	defer store.Close()
	pricer := pricer.New(logger, store, overlay, 10)

	kad := mockkad.NewMockKademlia(mockkad.WithDepth(2))
	pricer.SetKademlia(kad)

	peer := swarm.MustParseHexAddress("07e0d4ba628ad700fff48b16b52953487a22589362e34efdd7e0d4ba628ad700")

	requestHeaders := p2p.Headers{
		"irrelevantfield": []byte{},
	}

	responseHeaders := pricer.PriceHeadler(requestHeaders, peer)

	if responseHeaders["target"] != nil {
		t.Fatal("only error should be returned")
	}

	if responseHeaders["price"] != nil {
		t.Fatal("only error should be returned")
	}

	if responseHeaders["index"] != nil {
		t.Fatal("only error should be returned")
	}

	if responseHeaders["error"] == nil {
		t.Fatal("error should be returned")
	}
}
