// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint_test

import (
	"bytes"
	"errors"
	"math"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func FuzzParseSymbolData(f *testing.F) {
	r := newRand(40)
	m := fingerprint.NewFloatMapper()
	honest := fingerprint.AppendSymbolData(nil, m, 50, 0, encodeSet(m, randIDs(r, 50), 0, 8))
	f.Add(uint64(50), uint64(0), honest, false)
	f.Add(uint64(50), uint64(3), honest, true)
	f.Add(uint64(0), uint64(0), []byte{}, false)
	f.Add(uint64(math.MaxUint64), uint64(math.MaxUint64-1), honest[:fingerprint.MinSymbolSize], false)
	f.Add(uint64(7), uint64(math.MaxUint64), honest[:fingerprint.MinSymbolSize], false)
	f.Add(uint64(7), uint64(0), honest[:fingerprint.MinSymbolSize-1], false)
	f.Add(uint64(7), uint64(0), append(bytes.Clone(honest[:fingerprint.MinSymbolSize-1]), 0x80, 0x00), false)
	maxVarint := append(bytes.Clone(honest[:fingerprint.MinSymbolSize-1]), 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01)
	f.Add(uint64(1), uint64(9), maxVarint, true)
	f.Add(uint64(1), uint64(9), append(maxVarint[:len(maxVarint)-1], 0x02), true)

	f.Fuzz(func(t *testing.T, count, start uint64, data []byte, alt bool) {
		var mp fingerprint.Mapper = m
		if alt {
			mp = testMapper{}
		}
		syms, err := fingerprint.ParseSymbolData(mp, count, start, data)
		if err != nil {
			if !errors.Is(err, fingerprint.ErrInvalidSymbolData) {
				t.Fatalf("got %v, want ErrInvalidSymbolData", err)
			}
			return
		}
		n := uint64(len(syms))
		if n > math.MaxUint64-start {
			t.Fatalf("%d symbols from %d accepted", n, start)
		}
		if len(data) < len(syms)*fingerprint.MinSymbolSize || len(data) > len(syms)*fingerprint.MaxSymbolSize {
			t.Fatalf("%d symbols in %d bytes", len(syms), len(data))
		}
		// Only canonical data is accepted: it re-encodes byte for byte.
		if again := fingerprint.AppendSymbolData(nil, mp, count, start, syms); !bytes.Equal(again, data) {
			t.Fatalf("re-encoding differs:\n%x\n%x", again, data)
		}
	})
}

// FuzzDecoderAdd feeds the decoder a stream that is the honest symbol stream
// of a multiset of signed ids (duplicates allowed) plus arbitrary symbols
// parsed from junk, in batches of fuzzed sizes.
func FuzzDecoderAdd(f *testing.F) {
	r := newRand(41)
	var ids []byte
	for _, id := range randIDs(r, 5) {
		ids = append(ids, id[:]...)
	}
	m := fingerprint.NewFloatMapper()
	junk := fingerprint.AppendSymbolData(nil, m, 0, 0, []fingerprint.CodedSymbol{randSymbol(r), pureSymbol(randomID(r), 1)})
	f.Add(ids, []byte{}, []byte{3, 1, 7}, uint8(0))
	f.Add(ids, []byte{}, []byte{0}, uint8(2))
	f.Add(ids, junk, []byte{1, 2}, uint8(0))
	f.Add(append(bytes.Clone(ids), ids[:fingerprint.IDSize]...), []byte{}, []byte{15}, uint8(0))
	f.Add([]byte{}, junk, []byte{}, uint8(1))

	const n = 64
	f.Fuzz(func(t *testing.T, idBytes, junk, batches []byte, bits uint8) {
		var signed []fingerprint.ID
		for k := 0; k+fingerprint.IDSize <= len(idBytes) && len(signed) < 32; k += fingerprint.IDSize {
			signed = append(signed, fingerprint.ID(idBytes[k:k+fingerprint.IDSize]))
		}
		rg := fingerprint.NewRange(swarm.ZeroAddress, bits%4)

		// Even positions are only remote, odd ones only local.
		remote := make([]fingerprint.CodedSymbol, n)
		local := make([]fingerprint.CodedSymbol, n)
		var onlyLocal, onlyRemote []fingerprint.ID
		honest := true
		seen := map[fingerprint.ID]bool{}
		for k, id := range signed {
			if k%2 == 0 {
				fingerprint.Encode(m, remote, 0, id, 1)
				onlyRemote = append(onlyRemote, id)
			} else {
				fingerprint.Encode(m, local, 0, id, 1)
				onlyLocal = append(onlyLocal, id)
			}
			if seen[id] || !rg.ContainsID(id) {
				honest = false
			}
			seen[id] = true
		}
		if extra, err := fingerprint.ParseSymbolData(m, 0, 0, junk); err == nil && len(extra) > 0 {
			honest = false
			for k := range extra {
				remote[k%n].Add(&extra[k])
			}
		}

		dec := fingerprint.NewDecoder(m, rg)
		var sent uint64
		var err error
		for k := 0; sent < n; k++ {
			size := uint64(n)
			if k < len(batches) {
				size = uint64(batches[k] % 17)
			}
			end := min(sent+size, n)
			err = dec.Add(sent, remote[sent:end], local[sent:end])
			if err != nil || dec.Done() {
				break
			}
			sent = end
			if dec.Received() != sent {
				t.Fatalf("received %d, want %d", dec.Received(), sent)
			}
		}

		gotLocal, gotRemote := dec.Result()
		if len(gotLocal)+len(gotRemote) != dec.Recovered() {
			t.Fatal("Recovered differs from Result")
		}
		if uint64(dec.Recovered()) > 2*dec.Received() {
			t.Fatalf("%d ids recovered from %d symbols", dec.Recovered(), dec.Received())
		}
		all := map[fingerprint.ID]bool{}
		for _, id := range append(gotLocal, gotRemote...) {
			if all[id] {
				t.Fatal("id recovered twice")
			}
			all[id] = true
			if !rg.ContainsID(id) {
				t.Fatal("recovered id outside the range")
			}
		}
		if err != nil {
			if errors.Is(err, fingerprint.ErrInvalidBatch) {
				t.Fatalf("valid batch rejected: %v", err)
			}
			if !errors.Is(err, fingerprint.ErrCorrupt) {
				t.Fatalf("got %v, want ErrCorrupt", err)
			}
			if honest {
				t.Fatalf("honest stream reported corrupt: %v", err)
			}
			if dec.Done() {
				t.Fatal("corrupt decoder is done")
			}
			if err := dec.Add(dec.Received(), nil, nil); !errors.Is(err, fingerprint.ErrCorrupt) {
				t.Fatalf("corruption not sticky: %v", err)
			}
			return
		}
		if honest && dec.Done() && (!equalIDs(gotLocal, onlyLocal) || !equalIDs(gotRemote, onlyRemote)) {
			t.Fatal("honest stream decoded to a wrong result")
		}
	})
}
