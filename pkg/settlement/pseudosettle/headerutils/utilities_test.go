// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headerutils_test

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle/headerutils"
)

func TestMakeAllowanceResponseHeaders(t *testing.T) {

	makeHeaders, err := headerutils.MakeAllowanceResponseHeaders(big.NewInt(906000000000), int64(5348))
	if err != nil {
		t.Fatal(err)
	}

	expectedHeaders := p2p.Headers{
		headerutils.AllowanceFieldName: []byte{210, 241, 206, 228, 0},
		headerutils.TimestampFieldName: []byte{0, 0, 0, 0, 0, 0, 20, 228},
	}

	if !reflect.DeepEqual(makeHeaders, expectedHeaders) {
		t.Fatalf("Made headers not as expected, got %+v, want %+v", makeHeaders, expectedHeaders)
	}
}

func TestParseAllowanceHeaders(t *testing.T) {

	toReadHeaders := p2p.Headers{
		headerutils.AllowanceFieldName: []byte{234, 96},
		headerutils.TimestampFieldName: []byte{0, 0, 0, 0, 0, 0, 20, 228},
	}

	parsedAllowance, parsedTimestamp, err := headerutils.ParseAllowanceResponseHeaders(toReadHeaders)
	if err != nil {
		t.Fatal(err)
	}

	if parsedTimestamp != int64(5348) {
		t.Fatalf("Timestamp mismatch, got %v, want %v", parsedTimestamp, 5348)
	}

	if parsedAllowance.Cmp(big.NewInt(60000)) != 0 {
		t.Fatalf("Target mismatch, got %v, want %v", parsedAllowance, big.NewInt(60000))
	}
}

func TestParseAllowanceHeader(t *testing.T) {
	toReadHeaders := p2p.Headers{
		headerutils.AllowanceFieldName: []byte{210, 241, 206, 228, 0},
	}

	parsedAllowance, err := headerutils.ParseAllowanceHeader(toReadHeaders)
	if err != nil {
		t.Fatal(err)
	}

	if parsedAllowance.Cmp(big.NewInt(906000000000)) != 0 {
		t.Fatalf("Allowance mismatch, got %v, want %v", parsedAllowance, big.NewInt(906000000000))
	}

}

func TestParseTimestampHeader(t *testing.T) {
	toReadHeaders := p2p.Headers{
		headerutils.TimestampFieldName: []byte{0, 0, 0, 0, 0, 0, 20, 228},
	}

	parsedTimestamp, err := headerutils.ParseTimestampHeader(toReadHeaders)
	if err != nil {
		t.Fatal(err)
	}

	if parsedTimestamp != int64(5348) {
		t.Fatalf("Timestamp mismatch, got %v, want %v", parsedTimestamp, 5348)
	}

}

/*
func TestReadMalformedHeaders(t *testing.T) {
	toReadHeaders := p2p.Headers{
		headerutils.IndexFieldName:  []byte{11, 0},
		headerutils.TargetFieldName: []byte{1, 1, 1, 225, 1, 1, 1},
		headerutils.PriceFieldName:  []byte{0, 0, 0, 0, 0, 20, 228},
	}

	_, err := headerutils.ParseTimestampHeader(toReadHeaders)
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
*/
