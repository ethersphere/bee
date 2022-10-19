// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	storage "github.com/ethersphere/bee/pkg/storagev2"
)

type (
	// ItemDeleteFn is callback function called in migration step
	// to check if Item should be removed in this step.
	ItemDeleteFn func(storage.Item) (delete bool)

	// ItemUpdateFn is callback function called in migration step
	// to check if Item should be updated in this step.
	ItemUpdateFn func(storage.Item) (updateditem storage.Item, hasChanged bool)
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
	deleteFn ItemDeleteFn
	updateFn ItemUpdateFn
}

func newOptions() *options {
	return &options{
		deleteFn: func(storage.Item) bool { return false },
		updateFn: func(i storage.Item) (storage.Item, bool) { return i, false },
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
func NewStepOnIndex(query storage.Query, opts ...option) StepFn {
	o := newOptions()
	o.applyAll(opts)

	return func(s storage.Store) error {
		itemsForDelete := make([]storage.Item, 0)
		itemsForUpdate := make([]storage.Item, 0)

		err := s.Iterate(query, func(r storage.Result) (bool, error) {
			item := r.Entry

			if delete := o.deleteFn(item); delete {
				itemsForDelete = append(itemsForDelete, newKey(item.ID(), item.Namespace()))
				return false, nil
			}

			oldID := item.ID()
			if updatedItem, hadChanged := o.updateFn(item); hadChanged {
				// If ID of item has changed - we will need to remove old
				// item from store, because saving new item will not overwrite old
				if oldID != updatedItem.ID() {
					// We can't append item(r.Entry) nor updatedItem to the slice
					// because IDs have change (we need old ID). So we need to create new item which will mock
					// storage.Item interface by returning Key of item we want to remove
					itemsForDelete = append(itemsForDelete, newKey(oldID, item.Namespace()))
				}

				itemsForUpdate = append(itemsForUpdate, updatedItem)

				return false, nil
			}

			return false, nil
		})
		if err != nil {
			return err
		}

		for _, key := range itemsForDelete {
			if err := s.Delete(key); err != nil {
				return err
			}
		}

		for _, i := range itemsForUpdate {
			if err := s.Put(i); err != nil {
				return err
			}
		}

		return nil
	}
}

type key struct {
	storage.Marshaler
	storage.Unmarshaler

	id        string
	namespace string
}

func newKey(id, namespace string) *key {
	return &key{
		id:        id,
		namespace: namespace,
	}
}

func (k *key) ID() string        { return k.id }
func (k *key) Namespace() string { return k.namespace }
