// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headerutils_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/pricer/headerutils"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzParsePricingHeaders fuzzes the pricing-header parser, which runs on the
// peer-supplied stream headers of a pricing exchange. It transitively exercises
// ParseTargetHeader and ParsePriceHeader. The target asserts the parser never
// panics on arbitrary or missing fields.
func FuzzParsePricingHeaders(f *testing.F) {
	// valid production-built seed
	addr := swarm.NewAddress([]byte{
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
		0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
	})
	validHeaders, err := headerutils.MakePricingHeaders(42, addr)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(validHeaders[headerutils.PriceFieldName], validHeaders[headerutils.TargetFieldName])

	// benign edge seeds
	f.Add([]byte{}, []byte{})
	f.Add([]byte(nil), []byte(nil))
	f.Add([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}, addr.Bytes())             // 7-byte price
	f.Add([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09}, addr.Bytes()) // 9-byte price
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}, []byte{0xaa, 0xbb}) // non-32-byte target

	f.Fuzz(func(t *testing.T, price, target []byte) {
		headers := p2p.Headers{
			headerutils.PriceFieldName:  price,
			headerutils.TargetFieldName: target,
		}
		gotTarget, gotPrice, err := headerutils.ParsePricingHeaders(headers)
		if err != nil {
			return
		}
		// target round-trips exactly (swarm.NewAddress does no length validation)
		if !bytes.Equal(gotTarget.Bytes(), target) {
			t.Fatalf("target mismatch: got %x want %x", gotTarget.Bytes(), target)
		}
		// price parser only accepts len==8, so this always holds on success
		if len(price) != 8 {
			t.Fatalf("price accepted with length %d", len(price))
		}
		if want := binary.BigEndian.Uint64(price); gotPrice != want {
			t.Fatalf("price mismatch: got %d want %d", gotPrice, want)
		}
	})
}

// FuzzParsePricingResponseHeaders fuzzes the pricing-response-header parser,
// which runs on the peer-supplied stream headers of a pricing response. It
// transitively exercises ParseTargetHeader, ParsePriceHeader and
// ParseIndexHeader. The target asserts the parser never panics on arbitrary or
// missing fields.
func FuzzParsePricingResponseHeaders(f *testing.F) {
	// valid production-built seed
	addr := swarm.NewAddress([]byte{
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
		0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
	})
	validHeaders, err := headerutils.MakePricingResponseHeaders(42, addr, 7)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(
		validHeaders[headerutils.PriceFieldName],
		validHeaders[headerutils.TargetFieldName],
		validHeaders[headerutils.IndexFieldName],
	)

	// benign edge seeds
	f.Add([]byte{}, []byte{}, []byte{})
	f.Add([]byte(nil), []byte(nil), []byte(nil))
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}, addr.Bytes(), []byte{})           // empty index
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}, addr.Bytes(), []byte{0x01, 0x02}) // 2-byte index
	f.Add([]byte{0x01, 0x02, 0x03}, addr.Bytes(), []byte{0x05})                                     // wrong-length price
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}, []byte{0xaa, 0xbb}, []byte{0x05}) // non-32-byte target

	f.Fuzz(func(t *testing.T, price, target, index []byte) {
		headers := p2p.Headers{
			headerutils.PriceFieldName:  price,
			headerutils.TargetFieldName: target,
			headerutils.IndexFieldName:  index,
		}
		gotTarget, gotPrice, gotIndex, err := headerutils.ParsePricingResponseHeaders(headers)
		if err != nil {
			return
		}
		if !bytes.Equal(gotTarget.Bytes(), target) {
			t.Fatalf("target mismatch: got %x want %x", gotTarget.Bytes(), target)
		}
		if len(price) != 8 {
			t.Fatalf("price accepted with length %d", len(price))
		}
		if want := binary.BigEndian.Uint64(price); gotPrice != want {
			t.Fatalf("price mismatch: got %d want %d", gotPrice, want)
		}
		if len(index) != 1 {
			t.Fatalf("index accepted with length %d", len(index))
		}
		if gotIndex != index[0] {
			t.Fatalf("index mismatch: got %d want %d", gotIndex, index[0])
		}
	})
}
