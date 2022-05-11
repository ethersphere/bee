package potter_test

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/potter"
	"github.com/ethersphere/bee/pkg/potter/pot"
)

type game struct {
	time  int64
	name  string
	score uint64
}

func (g *game) String() string {
	return fmt.Sprintf("<%d, %s, %d>", g.time, g.name, g.score)
}

var features = []potter.Feature{
	{
		Name: "time",
		Size: 8,
		Decode: func(m potter.Model, b []byte) error {
			m.(*game).time = int64(binary.BigEndian.Uint64(b))
			return nil
		},
		Encode: func(m potter.Model, b []byte) error {
			binary.BigEndian.PutUint64(b, uint64(m.(*game).time))
			return nil
		},
	},
	{
		Name: "name",
		Size: 32,
		Decode: func(m potter.Model, b []byte) error {
			l := int(uint8(b[0]))
			m.(*game).name = string(b[1 : l+1])
			return nil
		},
		Encode: func(m potter.Model, b []byte) error {
			name := m.(*game).name
			b[0] = uint8(len(name))
			copy(b[1:], []byte(name))
			return nil
		},
	},
	{
		Name: "score",
		Size: 8,
		Decode: func(m potter.Model, b []byte) error {
			m.(*game).score = binary.BigEndian.Uint64(b)
			return nil
		},
		Encode: func(m potter.Model, b []byte) error {
			binary.BigEndian.PutUint64(b, uint64(m.(*game).score))
			return nil
		},
	},
}

var faces = []potter.Facet{
	{
		Name: "score.by.name.time",
		Key:  []string{"name", "time"},
		Val:  []string{"score"},
	},
	{
		Name: "by.score.name.time",
		Key:  []string{"score", "name", "time"},
		Val:  []string{},
	},
	{
		Name: "by.time",
		Key:  []string{"time"},
		Val:  []string{"score", "name"},
	},
}

func TestPotteryFind(t *testing.T) {
	dir := t.TempDir()
	schema := potter.NewSchema(features)
	p, err := potter.NewPottery(dir, schema, faces, testLogger)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.TODO()
	r := potter.NewRecord(&game{time.Now().Unix(), "shrek", 1000}, "time", "name", "score")
	t.Run("not found in empty pottery", func(t *testing.T) {
		if err = p.Find(ctx, "score.by.name.time", r); !errors.Is(err, pot.ErrNotFound) {
			t.Fatalf("expected pot.ErrNotFound, got %v", err)
		}
	})
	if err = p.Add(ctx, r); err != nil {
		t.Fatal(err)
	}
	r = potter.NewRecord(&game{time.Now().Unix(), "shrek", 1000}, "time", "name", "score")
	if err = p.Add(ctx, r); err != nil {
		t.Fatal(err)
	}
	t.Run("found in index score.by.name.time", func(t *testing.T) {
		if err = p.Find(ctx, "score.by.name.time", r); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("fill in unspecified field", func(t *testing.T) {
		if score := r.Model.(*game).score; score != 1000 {
			t.Fatalf("expected score to be '%v', got '%v'.", 1000, score)
		}
	})

	t.Run("found in index by.time", func(t *testing.T) {
		if err = p.Find(ctx, "by.time", r); err != nil {
			t.Fatal(err)
		}
	})

	k := potter.NewRecord(&game{time: r.Model.(*game).time})
	t.Run("partial model lookup", func(t *testing.T) {
		k.Set("time", "score")
		if err = p.Find(ctx, "by.time", k); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("not fill in specified field", func(t *testing.T) {
		if name := k.Model.(*game).name; name != "shrek" {
			t.Fatalf("expected name to be '%s', got '%s'.", "shrek", name)
		}
		if score := k.Model.(*game).score; score != 0 {
			t.Fatalf("expected score to be '%v', got '%v'.", 0, score)
		}
	})

	t.Run("fill in missing field", func(t *testing.T) {
		k.Unset("score")
		if err = p.Find(ctx, "by.time", k); err != nil {
			t.Fatal(err)
		}
		if score := k.Model.(*game).score; score != 1000 {
			t.Fatalf("expected name to be '%v', got '%v'.", 1000, score)
		}
	})
}

func TestPotteryFindAll(t *testing.T) {
	dir := t.TempDir()
	schema := potter.NewSchema(features)
	p, err := potter.NewPottery(dir, schema, faces, testLogger)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.TODO()
	now := time.Now().Unix()
	count := 3
	for i := 0; i < count; i++ {
		r := potter.NewRecord(&game{now + int64(i), "shrek", 1000}, "time", "name", "score")
		if err = p.Add(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	k := potter.NewRecord(&game{name: "shrek", score: 1000}, "name", "score")
	n := 0
	create := func() *potter.Record { return potter.NewRecord(&game{}) }
	if err = p.Iterate(ctx, "by.score.name.time", k, create(), create, func(r *potter.Record) (bool, error) {
		if score := r.Model.(*game).score; score != 1000 {
			t.Fatalf("expected score to be '%v', got '%v'.", 1000, score)
		}
		if name := r.Model.(*game).name; name != "shrek" {
			t.Fatalf("expected name to be '%v', got '%v'.", "shrek", name)
		}
		expTime := now + int64(n)
		if gotTime := r.Model.(*game).time; gotTime != expTime {
			t.Fatalf("expected time to be '%v', got '%v'.", expTime, gotTime)
		}
		n++
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	if n != int(count) {
		t.Fatalf("incorrect number of items. want %d, got %d", int(count), n)
	}

}
