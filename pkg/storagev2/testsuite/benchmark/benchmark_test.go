package test

import (
	"testing"
)

func BenchmarkInMemory(b *testing.B) {
	st, err := NewInMem()
	if err != nil {
		b.Fatal(err)
	}

	b.Cleanup(func() { _ = st.Close() })
	name := "in-mem"

	testSet(name, st)
	testGet(name, st)
	testGetSet(name, st)
	testDelete(name, st)
}

func BenchmarkLevelDB(b *testing.B) {
	tmp := b.TempDir()
	st, err := NewLevelDB(tmp, false)
	if err != nil {
		b.Fatal(err)
	}

	name := "levedb/nofsync"

	testSet(name, st)
	testGet(name, st)
	testGetSet(name, st)
	testDelete(name, st)

	_ = st.Close()

	tmp = b.TempDir()
	st, err = NewLevelDB(tmp, true)
	if err != nil {
		b.Fatal(err)
	}

	b.Cleanup(func() { _ = st.Close() })

	name = "levedb/fsync"
	testSet(name, st)
	testGet(name, st)
	testGetSet(name, st)
	testDelete(name, st)
}
