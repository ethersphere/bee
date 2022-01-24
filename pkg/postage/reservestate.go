// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

type ReserveState struct {
	Radius        uint8
	StorageRadius uint8
	Available     int64
}
