// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import "github.com/ethersphere/bee/v2/pkg/sharky"

type ReleasedSlots = releasedSlots

func (r *releasedSlots) Add(loc sharky.Location)           { r.add(loc) }
func (r *releasedSlots) Publish(limits []uint32)           { r.publish(limits) }
func (r *releasedSlots) Contains(loc sharky.Location) bool { return r.contains(loc) }
