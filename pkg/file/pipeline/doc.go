// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/* Package pipeline provides functionality for hashing pipelines needed to create different flavors of merkelised representations
of arbitrary data.
The interface exposes an io.Writer and Sum method, for components to use as a black box.
Within a pipeline, writers are chainable. It is up for the implementer to decide whether a writer calls the next writer.
Implementers should always implement the Sum method and call the next writer's Sum method (in case there is one), returning its result to the calling context.
*/
package pipeline
