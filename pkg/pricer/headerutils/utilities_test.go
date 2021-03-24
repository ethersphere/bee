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
		headerutils.PriceFieldName:  []byte{0, 0, 0, 0, 0, 0, 20, 228},
		headerutils.TargetFieldName: []byte{1, 1, 1, 225, 1, 1, 1},
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
		headerutils.PriceFieldName:  []byte{0, 0, 0, 0, 0, 0, 20, 228},
		headerutils.TargetFieldName: []byte{1, 1, 1, 225, 1, 1, 1},
		headerutils.IndexFieldName:  []byte{11},
	}

	if !reflect.DeepEqual(makeHeaders, expectedHeaders) {
		t.Fatalf("Made headers not as expected, got %+v, want %+v", makeHeaders, expectedHeaders)
	}

}

func TestParsePricingHeaders(t *testing.T) {

	toReadHeaders := p2p.Headers{
		headerutils.PriceFieldName:  []byte{0, 0, 0, 0, 0, 0, 20, 228},
		headerutils.TargetFieldName: []byte{1, 1, 1, 225, 1, 1, 1},
	}

	parsedTarget, parsedPrice, err := headerutils.ParsePricingHeaders(toReadHeaders)
	if err != nil {
		t.Fatal(err)
	}

	addr := swarm.MustParseHexAddress("010101e1010101")

	if parsedPrice != uint64(5348) {
		t.Fatalf("Price mismatch, got %v, want %v", parsedPrice, 5348)
	}

	if !parsedTarget.Equal(addr) {
		t.Fatalf("Target mismatch, got %v, want %v", parsedTarget, addr)
	}
}

func TestParsePricingResponseHeaders(t *testing.T) {

	toReadHeaders := p2p.Headers{
		headerutils.PriceFieldName:  []byte{0, 0, 0, 0, 0, 0, 20, 228},
		headerutils.TargetFieldName: []byte{1, 1, 1, 225, 1, 1, 1},
		headerutils.IndexFieldName:  []byte{11},
	}

	parsedTarget, parsedPrice, parsedIndex, err := headerutils.ParsePricingResponseHeaders(toReadHeaders)
	if err != nil {
		t.Fatal(err)
	}

	addr := swarm.MustParseHexAddress("010101e1010101")

	if parsedPrice != uint64(5348) {
		t.Fatalf("Price mismatch, got %v, want %v", parsedPrice, 5348)
	}

	if parsedIndex != uint8(11) {
		t.Fatalf("Price mismatch, got %v, want %v", parsedPrice, 5348)
	}

	if !parsedTarget.Equal(addr) {
		t.Fatalf("Target mismatch, got %v, want %v", parsedTarget, addr)
	}
}

func TestParseIndexHeader(t *testing.T) {
	toReadHeaders := p2p.Headers{
		headerutils.IndexFieldName: []byte{11},
	}

	parsedIndex, err := headerutils.ParseIndexHeader(toReadHeaders)
	if err != nil {
		t.Fatal(err)
	}

	if parsedIndex != uint8(11) {
		t.Fatalf("Index mismatch, got %v, want %v", parsedIndex, 11)
	}

}

func TestParseTargetHeader(t *testing.T) {

	toReadHeaders := p2p.Headers{
		headerutils.TargetFieldName: []byte{1, 1, 1, 225, 1, 1, 1},
	}

	parsedTarget, err := headerutils.ParseTargetHeader(toReadHeaders)
	if err != nil {
		t.Fatal(err)
	}

	addr := swarm.MustParseHexAddress("010101e1010101")

	if !parsedTarget.Equal(addr) {
		t.Fatalf("Target mismatch, got %v, want %v", parsedTarget, addr)
	}

}

func TestParsePriceHeader(t *testing.T) {
	toReadHeaders := p2p.Headers{
		headerutils.PriceFieldName: []byte{0, 0, 0, 0, 0, 0, 20, 228},
	}

	parsedPrice, err := headerutils.ParsePriceHeader(toReadHeaders)
	if err != nil {
		t.Fatal(err)
	}

	if parsedPrice != uint64(5348) {
		t.Fatalf("Index mismatch, got %v, want %v", parsedPrice, 5348)
	}

}

func TestReadMalformedHeaders(t *testing.T) {
	toReadHeaders := p2p.Headers{
		headerutils.IndexFieldName:  []byte{11, 0},
		headerutils.TargetFieldName: []byte{1, 1, 1, 225, 1, 1, 1},
		headerutils.PriceFieldName:  []byte{0, 0, 0, 0, 0, 20, 228},
	}

	_, err := headerutils.ParseIndexHeader(toReadHeaders)
	if err == nil {
		t.Fatal("Expected error from bad length of index bytes")
	}

	_, err = headerutils.ParsePriceHeader(toReadHeaders)
	if err == nil {
		t.Fatal("Expected error from bad length of price bytes")
	}

	_, _, _, err = headerutils.ParsePricingResponseHeaders(toReadHeaders)
	if err == nil {
		t.Fatal("Expected error caused by bad length of fields")
	}

	_, _, err = headerutils.ParsePricingHeaders(toReadHeaders)
	if err == nil {
		t.Fatal("Expected error caused by bad length of fields")
	}

}
