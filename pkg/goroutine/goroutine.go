// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package goroutine

import (
	"io"
	"runtime"
	"runtime/pprof"
)

func Dump(out io.Writer) {
	pprof.Lookup("goroutine").WriteTo(out, 1)
}

func Stack() string {
	buf := make([]byte, 1<<20)
	stacklen := runtime.Stack(buf, true)
	return string(buf[:stacklen])
}
