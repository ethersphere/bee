package shed

import (
	"context"
	"io/ioutil"
	"os"
	"testing"
)

// newTestDB is a helper function that constructs a
// temporary database and returns a cleanup function that must
// be called to remove the data.
func newTestDiskDB(t *testing.T) (db *DB, cleanupFunc func()) {
	t.Helper()

	dir, err := ioutil.TempDir("", "shed-test")
	if err != nil {
		t.Fatal(err)
	}
	db, err = NewDB(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err != nil {
		os.RemoveAll(dir)
		t.Fatal(err)
	}
	return db, func() {
		db.Store.Close(context.Background())
		os.RemoveAll(dir)
	}
}
