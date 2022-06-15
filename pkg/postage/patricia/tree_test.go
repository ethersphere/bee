package patricia_test

import (
	"encoding/json"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/ethersphere/bee/pkg/postage/patricia"
)

func TestPat(t *testing.T) {

	v := patricia.NewNode(nil)

	vt := patricia.NewNode([]byte{1, 5, 6})
	if !v.Insert(vt) {
		t.Fatal("did not insert")
	}
	vb := patricia.NewNode([]byte{1, 6, 9})
	if !v.Insert(vb) {
		t.Fatal("did not insert")
	}

	vj := patricia.NewNode([]byte{1, 5, 7})
	if !v.Insert(vj) {
		t.Fatal("did not insert")
	}

	vr := patricia.NewNode([]byte{1, 5, 7})
	if v.Insert(vr) {
		t.Fatal("should not insert")
	}
}

func TestMarshal(t *testing.T) {
	vs := patricia.NewNode(nil)
	vr := patricia.NewNode([]byte{1, 5, 7})
	vt := patricia.NewNode([]byte{1, 5, 9})
	vz := patricia.NewNode([]byte{1, 6, 7})

	vs.Insert(vr)
	vs.Insert(vt)
	vs.Insert(vz)

	bd, err := json.Marshal(vs)
	if err != nil {
		t.Fatal(err)
	}

	bdd := patricia.Node{}
	if err := json.Unmarshal(bd, &bdd); err != nil {
		t.Fatal(err)
	}
	spew.Dump(bdd)
}
