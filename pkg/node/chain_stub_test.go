// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/transaction"
)

var (
	_ p2p.SenderMatcher   = (*senderMatcherStub)(nil)
	_ chequebook.Service  = (*chequebookServiceStub)(nil)
	_ transaction.Backend = (*swapChainTransactionStub)(nil)
)
