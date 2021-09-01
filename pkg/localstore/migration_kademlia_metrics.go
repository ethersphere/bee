// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"time"
)

// DBSchemaKademliaMetrics is the bee schema identifier for kademlia metrics.
const DBSchemaKademliaMetrics = "kademlia-metrics"

// migrateKademliaMetrics removes all old existing
// kademlia metrics database content.
func migrateKademliaMetrics(db *DB) error {
	for _, prefix := range []string{"peer-last-seen-timestamp", "peer-total-connection-duration"} {
		start := time.Now()
		db.logger.Debugf("removing kademlia %q metrics", prefix)

		if err := db.stateStore.Iterate(prefix, func(k, _ []byte) (stop bool, err error) {
			key := string(k)
			//if err = db.stateStore.Delete(key); err != nil {
			//	return true, err
			//}
			db.logger.Debugf("deleted record: %", key)
			return false, nil
		}); err != nil {
			return err
		}

		db.logger.Debugf("removing kademlia %q metrics took %s", prefix, time.Since(start))
	}
	return nil
}
