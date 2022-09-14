// Copyright 2019 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

// DBSchemaCode is the first bee schema identifier.
const DBSchemaCode = "code"

// DBSchemaCurrent represents the DB schema we want to use.
// The actual/current DB schema might differ until migrations are run.
var DBSchemaCurrent = DBSchemaSharky
