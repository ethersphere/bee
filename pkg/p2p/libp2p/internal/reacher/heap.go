// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package reacher runs a background worker that will ping peers
// from an internal queue and report back the reachability to the notifier.
package reacher

// peerHeap is a min-heap of peers ordered by retryAfter time.
type peerHeap []*peer

func (h peerHeap) Len() int           { return len(h) }
func (h peerHeap) Less(i, j int) bool { return h[i].retryAfter.Before(h[j].retryAfter) }
func (h peerHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *peerHeap) Push(x any) {
	n := len(*h)
	p := x.(*peer)
	p.index = n
	*h = append(*h, p)
}

func (h *peerHeap) Pop() any {
	old := *h
	n := len(old)
	p := old[n-1]
	old[n-1] = nil // avoid memory leak
	p.index = -1   // for safety
	*h = old[0 : n-1]
	return p
}
