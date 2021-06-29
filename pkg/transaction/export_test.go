// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import "time"

var (
	StoredTransactionKey = storedTransactionKey
)

func (s *Matcher) SetTimeNow(f func() time.Time) {
	s.timeNow = f
}

func (s *Matcher) SetTime(k int64) {
	s.SetTimeNow(func() time.Time {
		return time.Unix(k, 0)
	})
}
