// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package depthmonitor

const DepthKey = depthKey

var (
	ManageWait = &manageWait
)

func (s *Service) StorageDepth() uint8 {
	s.depthLock.Lock()
	defer s.depthLock.Unlock()

	return s.storageDepth
}
