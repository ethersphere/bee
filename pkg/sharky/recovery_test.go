package sharky_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/sharky"
)

func TestRecovery(t *testing.T) {
	datasize := 4
	shards := 8
	shardSize := uint32(16)
	limit := shards * int(shardSize)

	dir := t.TempDir()
	s, err := sharky.New(dir, shards, shardSize, datasize)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	size := limit / 2
	data := make([]byte, 4)
	locs := make([]sharky.Location, size)
	for i := range locs {
		binary.BigEndian.PutUint32(data, uint32(i))
		loc, err := s.Write(ctx, data)
		if err != nil {
			t.Fatal(err)
		}
		locs[i] = loc
	}
	// extract locations to preserve / free in map
	preserved := make(map[uint32]bool)
	indexes := make([]uint32, size)
	for i := range indexes {
		indexes[i] = uint32(i)
	}
	rest := indexes[:]
	for n := size; n > size/2; n-- {
		i := rand.Intn(n)
		preserved[rest[i]] = false
		rest = append(rest[:i], rest[i+1:]...)
	}
	if len(rest) != len(preserved) {
		t.Fatalf("incorrect set sizes: %d <> %d", len(rest), len(preserved))
	}
	for _, i := range rest {
		preserved[i] = true
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// recover based on preserved map
	r, err := sharky.NewRecovery(dir, shards, shardSize, datasize)
	if err != nil {
		t.Fatal(err)
	}
	for i, add := range preserved {
		if add {
			if err := r.Add(locs[i]); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := r.Save(); err != nil {
		t.Fatal(err)
	}
	// check integrity of reecovered sharky
	s, err = sharky.New(dir, shards, shardSize, datasize)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	buf := make([]byte, datasize)
	t.Run("preserved are found", func(t *testing.T) {
		for i := range preserved {
			loc := locs[int(i)]
			if err := s.Read(ctx, loc, buf); err != nil {
				t.Fatal(err)
			}
			j := binary.BigEndian.Uint32(buf)
			if i != j {
				t.Fatalf("data not preserved at location %v: want %d; got %d", loc, i, j)
			}
		}
	})
	var freelocs []sharky.Location
	payload := []byte{0xff}
	t.Run("correct number of free slots", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		for {
			loc, err := s.Write(cctx, payload)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					break
				}
				t.Fatal(err)
			}
			freelocs = append(freelocs, loc)
		}
		if len(freelocs) != limit-size/2 {
			t.Fatalf("incorrect number of free slots: wanted %d; got %d", limit-size/2, len(freelocs))
		}
	})
	t.Run("added locs are still preserved", func(t *testing.T) {
		for i, added := range preserved {
			if !added {
				continue
			}
			if err := s.Read(ctx, locs[int(i)], buf); err != nil {
				t.Fatal(err)
			}
			j := binary.BigEndian.Uint32(buf)
			if i != j {
				t.Fatalf("data not preserved at location %v: want %d; got %d", locs[int(j)], i, j)
			}
		}
	})
	t.Run("not added preserved are overwritten", func(t *testing.T) {
		for i, added := range preserved {
			if added {
				continue
			}
			loc := locs[int(i)]
			loc.Length = 1
			if err := s.Read(ctx, loc, buf); err != nil {
				t.Fatal(err)
			}
			data := buf[:len(payload)]
			if !bytes.Equal(data, payload) {
				t.Fatalf("incorrect data on freed location %v: want %x; got %x", loc, payload, data)
			}
		}
	})
	t.Run("all other slots also overwritten", func(t *testing.T) {
		for _, loc := range freelocs {
			if err := s.Read(ctx, loc, buf); err != nil {
				t.Fatal(err)
			}
			data := buf[:len(payload)]
			if !bytes.Equal(data, payload) {
				t.Fatalf("incorrect data on freed location %v: want %x; got %x", loc, payload, data)
			}
		}
	})
}
