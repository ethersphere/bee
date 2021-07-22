// Copyright 2019 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package tags

import (
	"io/ioutil"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
)

var (
	allStates = []State{StateSplit, StateStored, StateSeen, StateSent, StateSynced}
)

// TestTagSingleIncrements tests if Inc increments the tag state value
func TestTagSingleIncrements(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	tg := &Tag{Total: 10, logger: logger}

	tc := []struct {
		state    uint32
		inc      int
		expcount int64
		exptotal int64
	}{
		{state: StateSplit, inc: 10, expcount: 10, exptotal: 10},
		{state: StateStored, inc: 9, expcount: 9, exptotal: 9},
		{state: StateSeen, inc: 1, expcount: 1, exptotal: 10},
		{state: StateSent, inc: 9, expcount: 9, exptotal: 9},
		{state: StateSynced, inc: 9, expcount: 9, exptotal: 9},
	}

	for _, tc := range tc {
		for i := 0; i < tc.inc; i++ {
			err := tg.Inc(tc.state)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	for _, tc := range tc {
		if tg.Get(tc.state) != tc.expcount {
			t.Fatalf("not incremented")
		}
	}
}

// TestTagStatus is a unit test to cover Tag.Status method functionality
func TestTagStatus(t *testing.T) {
	tg := &Tag{Total: 10}
	err := tg.Inc(StateSeen)
	if err != nil {
		t.Fatal(err)
	}
	err = tg.Inc(StateSent)
	if err != nil {
		t.Fatal(err)
	}
	err = tg.Inc(StateSynced)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		err = tg.Inc(StateSplit)
		if err != nil {
			t.Fatal(err)
		}
		err = tg.Inc(StateStored)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range []struct {
		state    State
		expVal   int64
		expTotal int64
	}{
		{state: StateStored, expVal: 10, expTotal: 10},
		{state: StateSplit, expVal: 10, expTotal: 10},
		{state: StateSeen, expVal: 1, expTotal: 10},
		{state: StateSent, expVal: 1, expTotal: 9},
		{state: StateSynced, expVal: 1, expTotal: 9},
	} {
		val, total, err := tg.Status(v.state)
		if err != nil {
			t.Fatal(err)
		}
		if val != v.expVal {
			t.Fatalf("should be %d, got %d", v.expVal, val)
		}
		if total != v.expTotal {
			t.Fatalf("expected Total to be %d, got %d", v.expTotal, total)
		}
	}
}

// tests ETA is precise
func TestTagETA(t *testing.T) {
	now := time.Now()
	maxDiff := 100000 // 100 microsecond
	tg := &Tag{Total: 10, StartedAt: now}
	time.Sleep(100 * time.Millisecond)
	err := tg.Inc(StateSplit)
	if err != nil {
		t.Fatal(err)
	}
	eta, err := tg.ETA(StateSplit)
	if err != nil {
		t.Fatal(err)
	}
	diff := time.Until(eta) - 9*time.Since(now)
	if int(diff) > maxDiff {
		t.Fatalf("ETA is not precise, got diff %v > .1ms", diff)
	}
}

// TestTagConcurrentIncrements tests Inc calls concurrently
func TestTagConcurrentIncrements(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	tg := &Tag{logger: logger}
	n := 10
	wg := sync.WaitGroup{}
	wg.Add(5 * n)
	for _, f := range allStates {
		go func(f State) {
			for j := 0; j < n; j++ {
				go func() {
					err := tg.Inc(f)
					if err != nil {
						t.Errorf("error incrementing tag counters: %v", err)
					}
					wg.Done()
				}()
			}
		}(f)
	}
	wg.Wait()
	for _, f := range allStates {
		v := tg.Get(f)
		if v != int64(n) {
			t.Fatalf("expected state %v to be %v, got %v", f, n, v)
		}
	}
}

// TestTagsMultipleConcurrentIncrements tests Inc calls concurrently
func TestTagsMultipleConcurrentIncrementsSyncMap(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	ts := NewTags(logger)
	n := 100
	wg := sync.WaitGroup{}
	wg.Add(10 * 5 * n)
	for i := 0; i < 10; i++ {
		tag, err := ts.Create(int64(n))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range allStates {
			go func(tag *Tag, f State) {
				for j := 0; j < n; j++ {
					go func() {
						err := tag.Inc(f)
						if err != nil {
							t.Errorf("error incrementing tag counters: %v", err)
						}
						wg.Done()
					}()
				}
			}(tag, f)
		}
	}
	wg.Wait()
	i := 0
	ts.Range(func(k, v interface{}) bool {
		i++
		uid := k.(uint32)
		for _, f := range allStates {
			tag, err := ts.Get(uid)
			if err != nil {
				t.Fatal(err)
			}
			stateVal := tag.Get(f)
			if stateVal != int64(n) {
				t.Fatalf("expected tag %v state %v to be %v, got %v", uid, f, n, v)
			}
		}
		return true

	})
	if i != 10 {
		t.Fatal("not enough tagz")
	}
}
