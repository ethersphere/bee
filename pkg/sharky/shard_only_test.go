package sharky

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestShard(t *testing.T) {
	shard := newShard(t)

	var loc Location
	var write0 = write{buf: []byte{0xff}, res: make(chan entry)}

	select {
	case shard.writes <- write0:
		e := <-write0.res
		if e.err != nil {
			t.Fatal("write entry", e.err)
		}
		loc = e.loc
		t.Log("wrote", write0.buf, "to slot", loc.Slot)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("write timeout")
	}

	buf := make([]byte, loc.Length)

	select {
	case shard.reads <- read{buf[:loc.Length], loc.Slot, 0}:
		if err := <-shard.errc; err != nil {
			t.Fatal("read", err)
		}
		t.Log("read", buf, "from slot", loc.Slot)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout reading")
	}

	ctx := context.Background()
	cctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	if err := shard.release(cctx, loc.Slot); err != nil {
		t.Fatal("release slot", loc.Slot, "err", err)
	}
	cancel()
	t.Log("released slot", loc.Slot)

	var write1 = write{buf: []byte{0x0e}, res: make(chan entry)}

	select {
	case shard.writes <- write1:
		e := <-write1.res
		if e.err != nil {
			t.Fatal("write entry", e.err)
		}
		loc = e.loc
		t.Log("wrote", write1.buf, "to slot", loc.Slot)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("write timeout")
	}

	select {
	case shard.reads <- read{buf[:loc.Length], loc.Slot, 0}:
		t.Log("read", buf, "from slot", loc.Slot)
		if err := <-shard.errc; err != nil {
			t.Fatal("read", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout reading")
	}
}

type dirFS string

func (d *dirFS) Open(path string) (fs.File, error) {
	return os.OpenFile(filepath.Join(string(*d), path), os.O_RDWR|os.O_CREATE, 0644)
}

func newShard(t *testing.T) *shard {
	t.Helper()

	basedir := dirFS(t.TempDir())
	index := 1

	file, err := basedir.Open(fmt.Sprintf("shard_%03d", index))
	if err != nil {
		t.Fatal(err)
	}

	ffile, err := basedir.Open(fmt.Sprintf("free_%03d", index))
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	sl := newSlots(ffile.(sharkyFile), &wg)
	err = sl.load()
	if err != nil {
		t.Fatal(err)
	}

	quit := make(chan struct{})
	sh := &shard{
		reads:    make(chan read),
		errc:     make(chan error),
		writes:   make(chan write),
		index:    uint8(index),
		datasize: 8,
		file:     file.(sharkyFile),
		slots:    sl,
		quit:     quit,
	}

	t.Cleanup(func() {
		quit <- struct{}{}
		if err := sh.close(); err != nil {
			t.Fatal("close shard", err)
		}
	})

	terminated := make(chan struct{})
	sh.slots.wg.Add(1)
	go func() {
		sh.process()
		close(terminated)
	}()

	go sl.process(terminated)

	return sh
}
