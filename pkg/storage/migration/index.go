// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"errors"
	"fmt"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
)

var ErrItemIDShouldntChange = errors.New("item.ID shouldn't be changing after update")

type (
	// ItemDeleteFn is callback function called in migration step
	// to check if Item should be removed in this step.
	ItemDeleteFn func(storage.Item) (deleted bool)

	// ItemUpdateFn is callback function called in migration step
	// to check if Item should be updated in this step.
	ItemUpdateFn func(storage.Item) (updatedItem storage.Item, hasChanged bool)
)

// WithItemDeleteFn return option with ItemDeleteFn set.
func WithItemDeleteFn(fn ItemDeleteFn) option {
	return func(o *options) {
		o.deleteFn = fn
	}
}

// WithItemUpdaterFn return option with ItemUpdateFn set.
func WithItemUpdaterFn(fn ItemUpdateFn) option {
	return func(o *options) {
		o.updateFn = fn
	}
}

type option func(*options)

type options struct {
	deleteFn   ItemDeleteFn
	updateFn   ItemUpdateFn
	opPerBatch int
}

func defaultOptions() *options {
	return &options{
		deleteFn:   func(storage.Item) bool { return false },
		updateFn:   func(i storage.Item) (storage.Item, bool) { return i, false },
		opPerBatch: 100,
	}
}

func (o *options) applyAll(opts []option) {
	for _, opt := range opts {
		opt(o)
	}
}

// NewStepOnIndex creates new migration step with update and/or delete operation.
// Migration will iterate on all elements selected by query and delete or update items
// based on supplied callback functions.
func NewStepOnIndex(s storage.BatchStore, query storage.Query, opts ...option) StepFn {
	o := defaultOptions()
	o.applyAll(opts)

	return func() error {
		return stepOnIndex(s, query, o)
	}
}

func stepOnIndex(s storage.Store, query storage.Query, o *options) error {
	var itemsForDelete, itemsForUpdate []storage.Item
	last := 0

	for {
		itemsForDelete = itemsForDelete[:0]
		itemsForUpdate = itemsForUpdate[:0]
		i := 0

		err := s.Iterate(query, func(r storage.Result) (bool, error) {
			if len(itemsForDelete)+len(itemsForUpdate) == o.opPerBatch {
				return true, nil
			}

			i++
			if i <= last {
				return false, nil
			}

			item := r.Entry

			if deleteItem := o.deleteFn(item); deleteItem {
				itemsForDelete = append(itemsForDelete, newKey(item))
				i--
				return false, nil
			}

			oldID := item.ID()
			if updatedItem, hadChanged := o.updateFn(item); hadChanged {
				if oldID != updatedItem.ID() {
					return true, ErrItemIDShouldntChange
				}

				itemsForUpdate = append(itemsForUpdate, updatedItem)
				return false, nil
			}

			return false, nil
		})
		if err != nil {
			return err
		}

		if err := deleteAll(s, itemsForDelete); err != nil {
			return err
		}

		if err := putAll(s, itemsForUpdate); err != nil {
			return err
		}

		if len(itemsForDelete) == 0 && len(itemsForUpdate) == 0 {
			break
		}

		last = i
	}

	return nil
}

func deleteAll(s storage.Store, items []storage.Item) error {
	for _, item := range items {
		if err := s.Delete(item); err != nil {
			return err
		}
	}

	return nil
}

func putAll(s storage.Store, items []storage.Item) error {
	for _, item := range items {
		if err := s.Put(item); err != nil {
			return err
		}
	}

	return nil
}

type key struct {
	storage.Marshaler
	storage.Unmarshaler
	storage.Cloner
	fmt.Stringer

	id        string
	namespace string
}

func newKey(k storage.Key) *key {
	return &key{
		id:        k.ID(),
		namespace: k.Namespace(),
	}
}

func (k *key) ID() string        { return k.id }
func (k *key) Namespace() string { return k.namespace }
