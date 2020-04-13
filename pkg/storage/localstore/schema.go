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

import (
	"github.com/ethersphere/swarm/log"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// The DB schema we want to use. The actual/current DB schema might differ
// until migrations are run.
var DbSchemaCurrent = DbSchemaDiwali

// There was a time when we had no schema at all.
const DbSchemaNone = ""

// "purity" is the first formal schema of LevelDB we release together with Swarm 0.3.5
const DbSchemaPurity = "purity"

// "halloween" is here because we had a screw in the garbage collector index.
// Because of that we had to rebuild the GC index to get rid of erroneous
// entries and that takes a long time. This schema is used for bookkeeping,
// so rebuild index will run just once.
const DbSchemaHalloween = "halloween"

const DbSchemaSanctuary = "sanctuary"

// the "diwali" migration simply renames the pullIndex in localstore
const DbSchemaDiwali = "diwali"

// returns true if legacy database is in the datadir
func IsLegacyDatabase(datadir string) bool {

	var (
		legacyDbSchemaKey = []byte{8}
	)

	db, err := leveldb.OpenFile(datadir, &opt.Options{OpenFilesCacheCapacity: 128})
	if err != nil {
		log.Error("got an error while trying to open leveldb path", "path", datadir, "err", err)
		return false
	}
	defer db.Close()

	data, err := db.Get(legacyDbSchemaKey, nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			// if we haven't found anything under the legacy db schema key- we are not on legacy
			return false
		}

		log.Error("got an unexpected error fetching legacy name from the database", "err", err)
	}
	log.Trace("checking if database scheme is legacy", "schema name", string(data))
	return string(data) == DbSchemaHalloween || string(data) == DbSchemaPurity
}
