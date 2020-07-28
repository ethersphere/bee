// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debouncer

import (
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"sync"
	"time"
)

type Interface interface {
	Put(reference string, value int64)
}

type Debouncer struct {
	logger logging.Logger
	store  storage.StateStorer
	lock   sync.Mutex
	set    map[string]int64
	wg     sync.WaitGroup
	quit   chan struct{} // quit channel
}

type Options struct {
	Logger logging.Logger
	Store  storage.StateStorer
}

func (d *Debouncer) Close() error {
	close(d.quit)
	d.wg.Wait()
	return nil
}

func NewDebouncer(o Options) *Debouncer {

	d := &Debouncer{
		logger: o.Logger,
		store:  o.Store,
		set:    make(map[string]int64),
		quit:   make(chan struct{}),
	}
	d.wg.Add(1)
	go d.manageWrites()

	return d

}

func (d *Debouncer) manageWrites() {

	for {
		select {
		case <-d.quit:
			d.writeSet()
			d.wg.Done()
			return
		case <-time.After(6 * time.Second):
			// periodically clear set
			d.writeSet()

		}
	}

}

func (d *Debouncer) writeSet() {
	d.lock.Lock()
	// use batch write from store
	copySet := d.set
	d.set = make(map[string]int64)
	d.lock.Unlock()
	//
	for reference, value := range copySet {
		err := d.store.Put(reference, value)
		if err != nil {
			d.logger.Errorf("error writing to store for peer %v in managed writes %v", reference, err)
		}
	}
}

func (d *Debouncer) Put(reference string, value int64) {
	d.lock.Lock()
	defer d.lock.Unlock()
	d.set[reference] = value
}
