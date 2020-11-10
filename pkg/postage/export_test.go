// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import "github.com/ethersphere/bee/pkg/swarm"

func (st *StampIssuer) Inc(a swarm.Address) error {
	return st.inc(a)
}
