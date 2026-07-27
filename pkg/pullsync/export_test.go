// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync

// Exported protocol identifiers for tests and fuzzers that need to open or
// register the pullsync stream through the p2p streamtest recorder.
const (
	ProtocolName    = protocolName
	ProtocolVersion = protocolVersion
	StreamName      = streamName
)
