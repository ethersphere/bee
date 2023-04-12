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
)

const maxDuration int64 = math.MaxInt64

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

	var (
		timer  *time.Timer
		timerC <-chan time.Time
	)

	for {
		select {
		case dur := <-l.durC:
			if timer == nil {
				timer = time.NewTimer(dur)
			} else {
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(dur)
			}
			timerC = timer.C
		case <-timerC:
			l.PruneExpiresAfter(0)
			l.mtx.Lock()
			if l.minHeap.Length() > 0 {
				timer.Reset(time.Until(time.Unix(0, l.minHeap.First())))
			}
			l.mtx.Unlock()
		case <-l.quit:
			return
		}
	}

}

func (l *List) Add(chunk, peer swarm.Address, expire time.Duration) {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	t := time.Now().Add(expire).UnixNano()

	heap.Push(l.minHeap, t)
	min := l.minHeap.First()

	if t == min { // pushed the most recent expiration
		select {
		case l.durC <- time.Until(time.Unix(0, min)):
		case <-l.quit:
		}
	}

	if _, ok := l.skip[chunk.ByteString()]; !ok {
		l.skip[chunk.ByteString()] = make(map[string]int64)
	}

	l.skip[chunk.ByteString()][peer.ByteString()] = t
}

func (l *List) AddForever(chunk, peer swarm.Address) {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	heap.Push(l.minHeap, maxDuration)

	if _, ok := l.skip[chunk.ByteString()]; !ok {
		l.skip[chunk.ByteString()] = make(map[string]int64)
	}

	l.skip[chunk.ByteString()][peer.ByteString()] = maxDuration
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

func (l *List) PruneExpiresAfter(d time.Duration) int {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	expiresNano := time.Now().Add(d).UnixNano()
	count := 0

	for l.minHeap.Length() > 0 && l.minHeap.First() <= expiresNano {
		heap.Pop(l.minHeap)
	}

	for k, chunkPeers := range l.skip {
		chunkPeersLen := len(chunkPeers)
		for peer, exp := range chunkPeers {
			if exp <= expiresNano {
				delete(chunkPeers, peer)
				count++
				chunkPeersLen--
			}
		}
		// prune the chunk too
		if chunkPeersLen == 0 {
			delete(l.skip, k)
		}
	}

	return count
}

func (l *List) Close() {
	close(l.quit)
	l.wg.Wait()

	l.mtx.Lock()
	for k := range l.skip {
		delete(l.skip, k)
	}
	l.mtx.Unlock()
}

// An IntHeap is a min-heap of ints.
type timeHeap []int64

func (h timeHeap) Len() int           { return len(h) }
func (h timeHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h timeHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *timeHeap) Push(x any) {
	*h = append(*h, x.(int64))
}

func (h *timeHeap) Length() int {
	return len(*h)
}

func (h *timeHeap) First() int64 {
	return (*h)[0]
}

func (h *timeHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
