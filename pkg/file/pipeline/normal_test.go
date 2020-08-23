package pipeline

import (
	"bytes"
	"fmt"
	"testing"

	test "github.com/ethersphere/bee/pkg/file/testing"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestNormalPipeline(t *testing.T) {
	m := mock.NewStorer()
	p := NewPipeline(m)
	data := []byte("hello world")
	_, _ = p.Write(data)
	sum, err := p.Sum()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal([]byte("92672a471f4419b255d7cb0cf313474a6f5856fb347c5ece85fb706d644b630f"), sum) {
		t.Fatal("error") // swarm (old) hash for hello world through bzz-raw is: 92672a471f4419b255d7cb0cf313474a6f5856fb347c5ece85fb706d644b630f
	}
}

func TestWrap(t *testing.T) {

	i := 17
	m := mock.NewStorer()
	p := NewPipeline(m)

	data, expect := test.GetVector(t, i)
	fmt.Println("vector length", len(data))
	_, _ = p.Write(data)
	sum, err := p.Sum()
	if err != nil {
		t.Fatal(err)
	}
	a := swarm.NewAddress(sum)
	if !a.Equal(expect) {
		t.Fatalf("failed run %d", i)
		t.Fatalf("expected address %s but got %s", expect.String(), a.String())
	}
	//fmt.Println("sum", hex.EncodeToString(sum))
}
func TestNormalPipelineWrapAll(t *testing.T) {
	for i := 0; i < 20; i++ {
		if i == 13 {
			continue
		}
		m := mock.NewStorer()
		p := NewPipeline(m)

		data, expect := test.GetVector(t, i)
		fmt.Println("vector length", len(data))
		_, _ = p.Write(data)
		sum, err := p.Sum()
		if err != nil {
			t.Fatal(err)
		}
		a := swarm.NewAddress(sum)
		if !a.Equal(expect) {
			t.Fatalf("failed run %d", i)
			t.Fatalf("expected address %s but got %s", expect.String(), a.String())
		}
		//fmt.Println("sum", hex.EncodeToString(sum))
	}
}
