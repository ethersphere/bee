package pipeline

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/pkg/storage/mock"
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
