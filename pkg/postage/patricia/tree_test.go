package patricia_test

import (
	"fmt"
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
	vr := patricia.NewNode([]byte{1, 5, 7, 8})
	vt := patricia.NewNode([]byte{1, 5, 9, 8})
	vz := patricia.NewNode([]byte{1, 6, 7, 7})

	if !vs.Insert(vr) {
		t.Fatal("expected to insert a node but didnt")
	}
	if !vs.Insert(vt) {
		t.Fatal("expected to insert a node but didnt")
	}

	if !vs.Insert(vz) {
		t.Fatal("expected to insert a node but didnt")
	}

	mm, err := vs.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(vs)
	fmt.Println(mm)
	//bd, err := json.Marshal(vs)
	//if err != nil {
	//t.Fatal(err)
	//}

	//bdd := patricia.Node{}
	//if err := json.Unmarshal(bd, &bdd); err != nil {
	//t.Fatal(err)
	//}
	//fmt.Println(string(bd))

	//if bdd.Insert(vr) {
	//t.Fatal("expected not to insert a node but did")
	//}

	//if bdd.Insert(vt) {
	//t.Fatal("expected not to insert a node but did")
	//}

	//if bdd.Insert(vz) {
	//t.Fatal("expected not to insert a node but did")
	//}
}
