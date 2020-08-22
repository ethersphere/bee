package pipeline

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"

	test "github.com/ethersphere/bee/pkg/file/testing"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestNormalPipeline(t *testing.T) {
	m := mock.NewStorer()
	p := NewPipeline(m)
	d := make([]byte, 8)
	binary.LittleEndian.PutUint64(d[:8], 11)
	data := append(d, []byte("hello world")...)
	_, _ = p.Write(data)
	sum, err := p.Sum()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("sum", hex.EncodeToString(sum))
	// swarm (old) hash for hello world through bzz-raw is:
	// 92672a471f4419b255d7cb0cf313474a6f5856fb347c5ece85fb706d644b630f
}

func TestNormalPipelineWrap(t *testing.T) {
	m := mock.NewStorer()
	p := NewPipeline(m)

	i := 7
	data, expect := test.GetVector(t, i)
	fmt.Println("vector length", len(data))
	_, _ = p.Write(data)
	sum, err := p.Sum()
	if err != nil {
		t.Fatal(err)
	}
	a := swarm.NewAddress(sum)
	if !a.Equal(expect) {
		t.Fatalf("expected address %s but got %s", expect.String(), a.String())
	}
	fmt.Println("sum", hex.EncodeToString(sum))
	// swarm (old) hash for hello world through bzz-raw is:
	// 92672a471f4419b255d7cb0cf313474a6f5856fb347c5ece85fb706d644b630f
}
