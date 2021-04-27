// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package p2p

import (
	"context"
	"errors"
	"time"
)

var ErrUnexpected = errors.New("unexpected request while in light mode")

// WithDisconnectStreams will mutate the given spec and replace the handler with a always erroring one.
func WithDisconnectStreams(spec ProtocolSpec) {
	for i := range spec.StreamSpecs {
		spec.StreamSpecs[i].Handler = func(c context.Context, p Peer, s Stream) error {
			return NewDisconnectError(ErrUnexpected)
		}
	}
}

// WithBlocklistStreams will mutate the given spec and replace the handler with a always erroring one.
func WithBlocklistStreams(dur time.Duration, spec ProtocolSpec) {
	for i := range spec.StreamSpecs {
		spec.StreamSpecs[i].Handler = func(c context.Context, p Peer, s Stream) error {
			return NewBlockPeerError(dur, ErrUnexpected)
		}
	}
}
