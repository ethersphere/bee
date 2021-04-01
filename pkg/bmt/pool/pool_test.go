// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pool_test

import (
	"sync"
	"testing"

	"github.com/ethersphere/bee/pkg/bmt/pool"
)

const str = "hello world"

func TestHasherPool(t *testing.T) {
	h := pool.New(3, 128)
	v := h.Get()
	_, err := v.Write([]byte(str))
	if err != nil {
		t.Fatal(err)
	}
	h.Put(v)
	if s := h.Size(); s != 1 {
		t.Fatalf("expected size 1 but got %d", s)
	}
}

func TestHasherPool_concurrent(t *testing.T) {
	h := pool.New(3, 128)

	c := make(chan struct{})
	var wg sync.WaitGroup

	// request 10 copies
	for i := 0; i < 10; i++ {
		v := h.Get()
		_, err := v.Write([]byte(str))
		if err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-c
			h.Put(v)
		}()
	}

	// when we get instances from the pool, we dont know
	// which ones are new and which aren't, so size is
	// only incremented when items are put back
	if s := h.Size(); s != 0 {
		t.Fatalf("expected size 0 but got %d", s)
	}

	close(c)
	wg.Wait()

	if s := h.Size(); s != 3 {
		t.Fatalf("expected size 3 but got %d", s)
	}
}
