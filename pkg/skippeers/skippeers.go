// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package skippeers

import (
	"container/heap"
	"math"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/exp/maps"
)

const MaxDuration time.Duration = math.MaxInt64

type List struct {
	mtx sync.Mutex

	minHeap *timeHeap

	durC chan time.Duration
	quit chan struct{}
	// key is chunk address, value is map of peer address to expiration
	skip map[string]map[string]int64

	wg sync.WaitGroup
}

func NewList() *List {
	l := &List{
		minHeap: &timeHeap{},
		skip:    make(map[string]map[string]int64),
		durC:    make(chan time.Duration),
		quit:    make(chan struct{}),
	}

	l.wg.Add(1)
	go l.worker()

	return l
}

func (l *List) worker() {

	defer l.wg.Done()

	timer := time.NewTimer(0)
	<-timer.C

	for {
		select {
		case dur := <-l.durC:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(dur)
		case <-timer.C:
			l.mtx.Lock()
			l.prune()
			if l.minHeap.Len() > 0 {
				timer.Reset(time.Until(time.Unix(0, l.minHeap.Peek())))
			}
			l.mtx.Unlock()
		case <-l.quit:
			return
		}
	}

}

func (l *List) Add(chunk, peer swarm.Address, expire time.Duration) {

	var t, min int64
	defer func() {
		// pushed the most recent timestamp
		if t == min {
			select {
			case l.durC <- time.Until(time.Unix(0, min)):
			case <-l.quit:
			}
		}
	}()

	l.mtx.Lock()
	defer l.mtx.Unlock()

	if expire == MaxDuration {
		t = MaxDuration.Nanoseconds()
	} else {
		t = time.Now().Add(expire).UnixNano()
		heap.Push(l.minHeap, t)
		min = l.minHeap.Peek()
	}

	if _, ok := l.skip[chunk.ByteString()]; !ok {
		l.skip[chunk.ByteString()] = make(map[string]int64)
	}

	l.skip[chunk.ByteString()][peer.ByteString()] = t
}

func (l *List) ChunkPeers(ch swarm.Address) (peers []swarm.Address) {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	now := time.Now().UnixNano()

	if p, ok := l.skip[ch.ByteString()]; ok {
		for peer, exp := range p {
			if exp > now {
				peers = append(peers, swarm.NewAddress([]byte(peer)))
			}
		}
	}
	return peers
}

func (l *List) PruneExpiresAfter(ch swarm.Address, d time.Duration) int {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	return l.pruneChunk(ch.ByteString(), time.Now().Add(d).UnixNano())
}

// Must be called under lock
func (l *List) prune() {
	expiresNano := time.Now().UnixNano()
	for k := range l.skip {
		l.pruneChunk(k, expiresNano)
	}
}

// Must be called under lock
func (l *List) pruneChunk(ch string, now int64) int {

	for l.minHeap.Len() > 0 && l.minHeap.Peek() <= now {
		heap.Pop(l.minHeap)
	}

	count := 0

	chunkPeers := l.skip[ch]
	chunkPeersLen := len(chunkPeers)
	for peer, exp := range chunkPeers {
		if exp <= now {
			delete(chunkPeers, peer)
			count++
			chunkPeersLen--
		}
	}

	// prune the chunk too
	if chunkPeersLen == 0 {
		delete(l.skip, ch)
	}

	return count
}

func (l *List) Close() error {
	close(l.quit)
	l.wg.Wait()

	l.mtx.Lock()
	maps.Clear(l.skip)
	l.mtx.Unlock()

	return nil
}

// An IntHeap is a min-heap of ints.
type timeHeap []int64

func (h timeHeap) Peek() int64        { return h[0] }
func (h timeHeap) Len() int           { return len(h) }
func (h timeHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h timeHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *timeHeap) Push(x any) { *h = append(*h, x.(int64)) }

func (h *timeHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
