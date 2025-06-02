// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wrapped

import (
	"github.com/ethersphere/bee/v2/pkg/transaction"
)

var (
	_ transaction.Backend = (*wrappedBackend)(nil)
)

func (b *wrappedBackend) Close() {
	b.backend.Close()
}
