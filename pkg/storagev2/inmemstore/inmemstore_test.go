package inmem_test

import (
	"testing"

	inmem "github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	storetesting "github.com/ethersphere/bee/pkg/storagev2/testsuite"
)

func TestStoreTestSuite(t *testing.T) {
	st := inmem.New()
	storetesting.RunTests(t, st)
}
