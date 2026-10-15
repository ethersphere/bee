// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync

import "github.com/ethersphere/bee/v2/pkg/pushsync/pb"

var (
	ProtocolName    = protocolName
	ProtocolVersion = protocolVersion
	StreamName      = streamName
)

// CheckReceipt exposes the unexported receipt verification (signature recovery,
// overlay derivation, proximity and shallow-receipt logic) for fuzzing.
func (ps *PushSync) CheckReceipt(receipt *pb.Receipt) error {
	return ps.checkReceipt(receipt)
}
