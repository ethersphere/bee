//go:build js
// +build js

// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import "io"

// Options specifies parameters that affect logger behavior.
type Options struct {
	sink       io.Writer
	verbosity  Level
	levelHooks levelHooks
	fmtOptions fmtOptions
}
