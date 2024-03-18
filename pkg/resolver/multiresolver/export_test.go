// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package multiresolver

import "github.com/ethersphere/bee/v2/pkg/log"

func GetLogger(mr *MultiResolver) log.Logger {
	return mr.logger
}

func GetCfgs(mr *MultiResolver) []ConnectionConfig {
	return mr.cfgs
}
