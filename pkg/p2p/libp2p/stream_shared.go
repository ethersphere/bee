// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"errors"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p"
)

var (
	closeDeadline  = 30 * time.Second
	errExpectedEof = errors.New("read: expected eof")
)
var _ p2p.Stream = (*stream)(nil)

func (s *stream) Headers() p2p.Headers {
	return s.headers
}

func (s *stream) ResponseHeaders() p2p.Headers {
	return s.responseHeaders
}
