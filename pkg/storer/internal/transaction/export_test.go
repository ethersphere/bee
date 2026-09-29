// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"

	"github.com/ethersphere/bee/v2/pkg/sharky"
)

type ReleasedSlots = releasedSlots

func (r *releasedSlots) Add(loc sharky.Location)           { r.add(loc) }
func (r *releasedSlots) Publish(limits []uint32)           { r.publish(limits) }
func (r *releasedSlots) Contains(loc sharky.Location) bool { return r.contains(loc) }

// SetAfterTableScan makes st call fn in NewSamplingView after the location
// table is built, before the releases seen so far are published.
func SetAfterTableScan(st Storage, fn func()) { st.(*store).afterTableScan = fn }

// SetSamplingViewAfterRead makes view call fn after each direct sharky read,
// before it checks whether the slot was released.
func SetSamplingViewAfterRead(view SamplingView, fn func()) {
	v := view.(*samplingView)
	read := v.read
	v.read = func(ctx context.Context, loc sharky.Location, buf []byte) error {
		err := read(ctx, loc, buf)
		fn()
		return err
	}
}
