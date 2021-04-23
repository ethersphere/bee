// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pseudosettle

import (
	"time"
)

const (
	AllowanceFieldName = allowanceFieldName
	TimestampFieldName = timestampFieldName
)

func (s *Service) SetTimeNow(f func() time.Time) {
	s.timeNow = f
}

func (s *Service) SetTime(k int64) {
	s.SetTimeNow(func() time.Time {
		return time.Unix(k, 0)
	})
}
