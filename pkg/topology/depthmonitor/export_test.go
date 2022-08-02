// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package depthmonitor

var (
	ManageWait = &manageWait
)

func (s *Service) StorageDepth() uint8 {
	return s.bs.GetReserveState().StorageRadius
}

func (s *Service) SetStorageRadius(r uint8) {
	_ = s.bs.SetStorageRadius(func(_ uint8) uint8 {
		return r
	})
}
