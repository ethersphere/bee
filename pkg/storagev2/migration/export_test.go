// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

func NewOptions() *options {
	return newOptions()
}

func (o *options) DeleteFn() ItemDeleteFn {
	return o.deleteFn
}

func (o *options) UpdateFn() ItemUpdateFn {
	return o.updateFn
}

func (o *options) ApplyAll(opt ...option) {
	o.applyAll(opt)
}
