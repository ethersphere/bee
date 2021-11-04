package franky

import (
	"bytes"
	"os"
	"testing"

	testingc "github.com/ethersphere/bee/pkg/storage/testing"
)

func TestCreate(t *testing.T) {
	dir := t.TempDir()
	sh, err := newShard(0, 10, 4096, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sh.close(); err != nil {
		t.Fatal(err)
	}
	f, err := os.Stat(fname(0, dir))
	if err != nil {
		t.Fatal(err)
	}

	if f.Size() != 10*4096 {
		t.Fatal(f.Size())
	}
}

func TestRW(t *testing.T) {
	dir := t.TempDir()
	cap := 16
	sh, err := newShard(0, int64(cap), 4096, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sh.close()

	for i := cap; i > 0; i-- {
		ch := testingc.GenerateTestRandomChunk()
		o, _ := sh.freeOffset()
		if o == -1 {
			t.Fatal("no offset found")
		}
		if err = sh.writeAt(ch.Data(), o); err != nil {
			t.Fatal(err)
		}
		data, err := sh.readAt(o)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, ch.Data()) {
			t.Fatal("data mismatch")
		}
	}

	// next write should fail
	o, _ := sh.freeOffset()
	if o != -1 {
		t.Fatal("expected no offsets found")
	}

	// release 1 offset
	sh.release(1)

	// next free offset should be 1
	o, _ = sh.freeOffset()
	if o != 1 {
		t.Fatalf("expected offset 1 got %d", o)
	}
}

func TestOffsetPersistence(t *testing.T) {
	dir := t.TempDir()
	cap := 16
	sh, err := newShard(0, int64(cap), 4096, dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	for i := cap; i > 0; i-- {
		o, _ := sh.freeOffset()
		if o == -1 {
			t.Fatal("no offset found")
		}
	}

	// next write should fail
	o, _ := sh.freeOffset()
	if o != -1 {
		t.Fatal("expected no offsets found")
	}

	// release 1 offset
	sh.release(1)

	// next free offset should be 1
	o, ff := sh.freeOffset()
	if o != 1 {
		t.Fatalf("expected offset 1 got %d", o)
	}

	ff() //fail so that the offset is reclaimed
	oo, err := sh.close()
	if err != nil {
		t.Fatal(err)
	}

	sh2, err := newShard(0, int64(cap), 4096, dir, oo)
	if err != nil {
		t.Fatal(err)
	}
	o, _ = sh2.freeOffset()
	if o != 1 {
		t.Fatalf("expected offset 1 got %d", o)
	}
}
