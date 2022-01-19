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

	payload := write{buf: []byte{0xff}, res: make(chan entry)}

	loc := writePayload(t, shard, payload)
	t.Log("wrote", payload.buf, "to slot", loc.Slot)

	buf := readFromLocation(t, shard, loc)
	t.Log("read", buf, "from slot", loc.Slot)

	releaseSlot(t, shard, loc)
	t.Log("released slot", loc.Slot)

	payload = write{buf: []byte{0xff >> 1}, res: make(chan entry)}

	loc = writePayload(t, shard, payload)
	t.Log("wrote", payload.buf, "to slot", loc.Slot)

	buf = readFromLocation(t, shard, loc)
	t.Log("read", buf, "from slot", loc.Slot)

	releaseSlot(t, shard, loc)
	t.Log("released slot", loc.Slot)

	i, runs := 0, 10
	for ; i < runs; i++ { // we write until slot 0 is release and available for writing
		payload = write{buf: []byte{0x01 << i}, res: make(chan entry)}
		loc = writePayload(t, shard, payload)
		releaseSlot(t, shard, loc)
		t.Log("wrote", payload.buf, "to slot", loc.Slot)
		if loc.Slot == 0 {
			t.Log("took", i, "iteration(s) to write back to slot", loc.Slot)
			break
		}
	}

	if i == runs {
		t.Errorf("expected to write to slot 0 in fewer than %d runs, but got %d runs", runs, i)
	}
}

func writePayload(t *testing.T, shard *shard, payload write) (loc Location) {
	t.Helper()

	shard.slots.wg.Add(1)

	select {
	case shard.writes <- payload:
		e := <-payload.res
		if e.err != nil {
			t.Fatal("write entry", e.err)
		}
		loc = e.loc
	case <-time.After(100 * time.Millisecond):
		t.Fatal("write timeout")
	}

	return loc
}

func readFromLocation(t *testing.T, shard *shard, loc Location) []byte {
	t.Helper()
	buf := make([]byte, loc.Length)

	select {
	case shard.reads <- read{buf[:loc.Length], loc.Slot, 0}:
		if err := <-shard.errc; err != nil {
			t.Fatal("read", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout reading")
	}

	return buf
}

func releaseSlot(t *testing.T, shard *shard, loc Location) {
	t.Helper()
	ctx := context.Background()
	cctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	if err := shard.release(cctx, loc.Slot); err != nil {
		t.Fatal("release slot", loc.Slot, "err", err)
	}
	cancel()
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

	shard.slots.wg.Add(1)
	go slots.process(terminated)

	return shard
}
