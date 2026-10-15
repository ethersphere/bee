// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package fingerprint holds the in-memory fingerprint index of the rangesync
protocol and the value types shared with it.

An element of the reserve is identified by an ID, the chunk address followed by
the stamp hash. Elements are grouped into Ranges, which are address prefixes,
and a Range is summarized by a Summary: the number of elements in it and a
digest of their LtHash.

A Range is also encoded as a Rateless IBLT stream of CodedSymbols: Encode adds
an element to a stream, a Decoder peels the difference of two streams, and
AppendSymbolData and ParseSymbolData are the wire codec. A Mapper, by default
a FloatMapper, decides which symbols an element is mapped to.

The package is the leaf of the rangesync tree: it depends on neither
pkg/storer nor pkg/rangesync, so the storer only exposes iteration and an
observer hook, and the protocol converts to and from storer types at its
boundary.
*/
package fingerprint
