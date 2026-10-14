// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package fingerprint holds the in-memory fingerprint index of the rangesync
protocol and the value types shared with it.

An element of the reserve is identified by an ID, the chunk address followed by
the stamp hash. Elements are grouped into Ranges, which are address prefixes,
and a Range is summarised by a Summary: the number of elements in it and a
digest of their LtHash.

The package is the leaf of the rangesync tree: it depends on neither
pkg/storer nor pkg/rangesync, so the storer only exposes iteration and an
observer hook, and the protocol converts to and from storer types at its
boundary.
*/
package fingerprint
