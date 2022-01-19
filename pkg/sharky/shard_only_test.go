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
	payload := write{buf: []byte{0xff}, res: make(chan entry)}

	shard.slots.wg.Add(1)

	select {
	case shard.writes <- payload:
		e := <-payload.res
		if e.err != nil {
			t.Fatal("write entry", e.err)
		}
		loc = e.loc
		t.Log("wrote", payload.buf, "to slot", loc.Slot)
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

	payload = write{buf: []byte{0xff >> 1}, res: make(chan entry)}

	shard.slots.wg.Add(1)

	select {
	case shard.writes <- payload:
		e := <-payload.res
		if e.err != nil {
			t.Fatal("write entry", e.err)
		}
		loc = e.loc
		t.Log("wrote", payload.buf, "to slot", loc.Slot)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("write timeout")
	}

	buf = make([]byte, loc.Length)

	select {
	case shard.reads <- read{buf[:loc.Length], loc.Slot, 0}:
		if err := <-shard.errc; err != nil {
			t.Fatal("read", err)
		}
		t.Log("read", buf, "from slot", loc.Slot)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout reading")
	}

	payload = write{buf: []byte{0xff >> 2}, res: make(chan entry)}

	shard.slots.wg.Add(1)

	select {
	case shard.writes <- payload:
		e := <-payload.res
		if e.err != nil {
			t.Fatal("write entry", e.err)
		}
		loc = e.loc
		t.Log("wrote", payload.buf, "to slot", loc.Slot)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("write timeout")
	}

	ctx = context.Background()
	cctx, cancel = context.WithTimeout(ctx, 500*time.Millisecond)
	if err := shard.release(cctx, loc.Slot); err != nil {
		t.Fatal("release slot", loc.Slot, "err", err)
	}
	cancel()
	t.Log("released slot", loc.Slot)

	buf = make([]byte, loc.Length)

	select {
	case shard.reads <- read{buf[:loc.Length], loc.Slot, 0}:
		if err := <-shard.errc; err != nil {
			t.Fatal("read", err)
		}
		t.Log("read", buf, "from slot", loc.Slot)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout reading")
	}
}

// 10 writes - 9 chunks

//  2 3 5 7

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

	slots := newSlots(ffile.(sharkyFile), &wg)
	err = slots.load()
	if err != nil {
		t.Fatal(err)
	}

	quit := make(chan struct{})
	shard := &shard{
		reads:    make(chan read),
		errc:     make(chan error),
		writes:   make(chan write),
		index:    uint8(index),
		datasize: 1,
		file:     file.(sharkyFile),
		slots:    slots,
		quit:     quit,
	}

	t.Cleanup(func() {
		quit <- struct{}{}
		if err := shard.close(); err != nil {
			t.Fatal("close shard", err)
		}
	})

	terminated := make(chan struct{})

	go func() {
		shard.process()
		close(terminated)
	}()

	go slots.process(terminated)

	return shard
}
