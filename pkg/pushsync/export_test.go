// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync

import (
	"context"

	"github.com/ethersphere/bee/v2/pkg/pushsync/pb"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	ProtocolName    = protocolName
	ProtocolVersion = protocolVersion
	StreamName      = streamName
)

// CheckReceipt exposes the unexported receipt verification (signature recovery,
// overlay derivation, proximity and shallow-receipt logic) for fuzzing.
func (ps *PushSync) CheckReceipt(receipt *pb.Receipt) error {
	return ps.checkReceipt(context.Background(), receipt)
}

// ReceiptNetworkCheckSkipped exposes the counter of receipts checked without
// the network radius.
func (ps *PushSync) ReceiptNetworkCheckSkipped() prometheus.Counter {
	return ps.metrics.ReceiptNetworkCheckSkipped
}
