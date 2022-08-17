package leveldbstore_test

import (
	"testing"

	inmem "github.com/ethersphere/bee/pkg/storagev2/leveldb"
	storetesting "github.com/ethersphere/bee/pkg/storagev2/testsuite"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func TestStoreTestSuite(t *testing.T) {
	dir := t.TempDir()
	st, err := inmem.NewLevelDBStore(dir, new(opt.Options))
	if err != nil {
		t.Fatal(err)
	}
	storetesting.RunCorrectnessTests(t, st)
}
