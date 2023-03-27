// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

var (
	ErrStorageVersionItemUnmarshalInvalidSize = errStorageVersionItemUnmarshalInvalidSize

	SetVersion = setVersion
)

func DefaultOptions() *options {
	return defaultOptions()
}

func (o *options) DeleteFn() ItemDeleteFn {
	return o.deleteFn
}

func (o *options) UpdateFn() ItemUpdateFn {
	return o.updateFn
}

func (o *options) OpPerBatch() int {
	return o.opPerBatch
}

func (o *options) ApplyAll(opt ...option) {
	o.applyAll(opt)
}

func WithOpPerBatch(count int) option {
	return func(o *options) {
		o.opPerBatch = count
	}
}
