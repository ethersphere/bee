// Copyright 2019 The Swarm Authors
// This file is part of the Swarm library.
//
// The Swarm library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Swarm library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Swarm library. If not, see <http://www.gnu.org/licenses/>.

package localstore

// The DB schema we want to use. The actual/current DB schema might differ
// until migrations are run.
var DbSchemaCurrent = DbSchemaYuj

// There was a time when we had no schema at all.
const DbSchemaNone = ""

// DbSchemaCode is the first bee schema identifier
const DbSchemaCode = "code"

// DbSchemaYuj is the bee schema indentifier for storage incentives
// initial iteration.
const DbSchemaYuj = "yuj"
