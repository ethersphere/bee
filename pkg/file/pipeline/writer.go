// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipeline

import "io"

// chainWriter is a writer in a pipeline.
// It is up to the implementer to decide whether a writer
// calls the next writer or not. Implementers should
// call the Sum method of the subsequent writer in case there
// exists one.
type chainWriter interface {
	chainWrite(*pipeWriteArgs) error
	sum() ([]byte, error)
}

// Interface exposes an `io.Writer` and `Sum` method, for components to use as a black box.
// Within a pipeline, writers are chainable. It is up for the implementer to decide whether
// a writer calls the next writer. Implementers should always implement the `Sum` method
// and call the next writer's `Sum` method (in case there is one), returning its result to
// the calling context.
type Interface interface {
	io.Writer
	Sum() ([]byte, error)
}
