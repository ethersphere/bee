// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headerutils_test

import (
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/pricer/headerutils"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestMakePricingHeaders(t *testing.T) {

	addr := swarm.MustParseHexAddress("010101e1010101")

	makeHeaders, err := headerutils.MakePricingHeaders(uint64(5348), addr)
	if err != nil {
		t.Fatal(err)

	}

	expectedHeaders := p2p.Headers{
		"price":  []byte{0, 0, 0, 0, 0, 0, 20, 228},
		"target": []byte{1, 1, 1, 225, 1, 1, 1},
	}

	if !reflect.DeepEqual(makeHeaders, expectedHeaders) {
		t.Fatalf("Made headers not as expected, got %+v, want %+v", makeHeaders, expectedHeaders)
	}

}

func TestMakePricingResponseHeaders(t *testing.T) {

	addr := swarm.MustParseHexAddress("010101e1010101")

	makeHeaders, err := headerutils.MakePricingResponseHeaders(uint64(5348), addr, uint8(11))
	if err != nil {
		t.Fatal(err)
	}

	expectedHeaders := p2p.Headers{
		"price":  []byte{0, 0, 0, 0, 0, 0, 20, 228},
		"target": []byte{1, 1, 1, 225, 1, 1, 1},
		"index":  []byte{11},
	}

	if !reflect.DeepEqual(makeHeaders, expectedHeaders) {
		t.Fatalf("Made headers not as expected, got %+v, want %+v", makeHeaders, expectedHeaders)
	}

}

func TestReadPricingHeaders(t *testing.T) {

	toReadHeaders := p2p.Headers{
		"price":  []byte{0, 0, 0, 0, 0, 0, 20, 228},
		"target": []byte{1, 1, 1, 225, 1, 1, 1},
	}

	readTarget, readPrice, err := headerutils.ReadPricingHeaders(toReadHeaders)
	if err != nil {
		t.Fatal(err)
	}

	addr := swarm.MustParseHexAddress("010101e1010101")

	if readPrice != uint64(5348) {
		t.Fatalf("Price mismatch, got %v, want %v", readPrice, 5348)
	}

	if !reflect.DeepEqual(readTarget, addr) {
		t.Fatalf("Target mismatch, got %v, want %v", readTarget, addr)
	}
}

func TestReadPricingResponseHeaders(t *testing.T) {

	toReadHeaders := p2p.Headers{
		"price":  []byte{0, 0, 0, 0, 0, 0, 20, 228},
		"target": []byte{1, 1, 1, 225, 1, 1, 1},
		"index":  []byte{11},
	}

	readTarget, readPrice, readIndex, err := headerutils.ReadPricingResponseHeaders(toReadHeaders)
	if err != nil {
		t.Fatal(err)
	}

	addr := swarm.MustParseHexAddress("010101e1010101")

	if readPrice != uint64(5348) {
		t.Fatalf("Price mismatch, got %v, want %v", readPrice, 5348)
	}

	if readIndex != uint8(11) {
		t.Fatalf("Price mismatch, got %v, want %v", readPrice, 5348)
	}

	if !reflect.DeepEqual(readTarget, addr) {
		t.Fatalf("Target mismatch, got %v, want %v", readTarget, addr)
	}
}

func TestReadIndexHeader(t *testing.T) {
	toReadHeaders := p2p.Headers{
		"index": []byte{11},
	}

	readIndex, err := headerutils.ReadIndexHeader(toReadHeaders)
	if err != nil {
		t.Fatal(err)
	}

	if readIndex != uint8(11) {
		t.Fatalf("Index mismatch, got %v, want %v", readIndex, 11)
	}

}

func TestReadTargetHeader(t *testing.T) {

	toReadHeaders := p2p.Headers{
		"target": []byte{1, 1, 1, 225, 1, 1, 1},
	}

	readTarget, err := headerutils.ReadTargetHeader(toReadHeaders)
	if err != nil {
		t.Fatal(err)
	}

	addr := swarm.MustParseHexAddress("010101e1010101")

	if !reflect.DeepEqual(readTarget, addr) {
		t.Fatalf("Target mismatch, got %v, want %v", readTarget, addr)
	}

}

func TestReadPriceHeader(t *testing.T) {
	toReadHeaders := p2p.Headers{
		"price": []byte{0, 0, 0, 0, 0, 0, 20, 228},
	}

	readPrice, err := headerutils.ReadPriceHeader(toReadHeaders)
	if err != nil {
		t.Fatal(err)
	}

	if readPrice != uint64(5348) {
		t.Fatalf("Index mismatch, got %v, want %v", readPrice, 5348)
	}

}

func TestReadMalformedHeaders(t *testing.T) {
	toReadHeaders := p2p.Headers{
		"index":  []byte{11, 0},
		"target": []byte{1, 1, 1, 225, 1, 1, 1},
		"price":  []byte{0, 0, 0, 0, 0, 20, 228},
	}

	_, err := headerutils.ReadIndexHeader(toReadHeaders)
	if err == nil {
		t.Fatal("Expected error from bad length of index bytes")
	}

	_, err = headerutils.ReadPriceHeader(toReadHeaders)
	if err == nil {
		t.Fatal("Expected error from bad length of price bytes")
	}

	_, _, _, err = headerutils.ReadPricingResponseHeaders(toReadHeaders)
	if err == nil {
		t.Fatal("Expected error caused by bad length of fields")
	}

	_, _, err = headerutils.ReadPricingHeaders(toReadHeaders)
	if err == nil {
		t.Fatal("Expected error caused by bad length of fields")
	}

}
