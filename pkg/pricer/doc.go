// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pricer exposes the main types and functions which tells the node the price it charges for certain chunks and
// the price it is charged by it's peers for chunks.
// A crucial variable in this package is the priceTable, which defines the prices for al chunks by setting a price at each index,
// where the index corresponds to a proximityOrder distance between the node and the chunk,
// up to the neighborhood depth of the node.
// The price differentiates per proximityOrder, because for further-away chunks, there is more effort needed to find the natural location of the chunk.
package pricer
