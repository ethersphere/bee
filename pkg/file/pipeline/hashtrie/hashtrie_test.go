package hashtrie_test

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/file/pipeline/bmt"
	"github.com/ethersphere/bee/pkg/file/pipeline/hashtrie"
	"github.com/ethersphere/bee/pkg/file/pipeline/store"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

const oneLevel = 2

var addr swarm.Address

func init() {
	b := make([]byte, 32)
	b[31] = 0x01
	addr = swarm.NewAddress(b)
}

func TestTwoLevels(t *testing.T) {
	s := mock.NewStorer()
	ctx := context.Background()
	mode := storage.ModePutUpload
	pf := func() pipeline.ChainWriter {
		lsw := store.NewStoreWriter(ctx, s, mode, nil)
		return bmt.NewBmtWriter(lsw)
	}

	ht := hashtrie.NewHashTrieWriter(64, 2, 32, pf)
	spb := make([]byte, 8)
	binary.LittleEndian.PutUint64(spb, 1)
	// here we need to write 3 chunks or 4 chunks, then
	// we get a rollup for level 1, 2, which then rolls up to 3
	for i := 0; i < 3; i++ {
		a := &pipeline.PipeWriteArgs{Ref: addr.Bytes(), Span: spb}
		ht.ChainWrite(a)
	}

	ref, err := ht.Sum()
	if err != nil {
		t.Fatal(err)
	}
	exphash := "281907c4199cd2b05b80469d2af5be103cc1317dcf78d5d9b95694aadb2d4994"
	if a := hex.EncodeToString(ref); a != exphash {
		t.Fatalf("expected hash %s but got %s", exphash, a)
	}
}
func TestOneLevel(t *testing.T) {
	s := mock.NewStorer()
	ctx := context.Background()
	mode := storage.ModePutUpload
	pf := func() pipeline.ChainWriter {
		lsw := store.NewStoreWriter(ctx, s, mode, nil)
		return bmt.NewBmtWriter(lsw)
	}

	ht := hashtrie.NewHashTrieWriter(64, 2, 32, pf)
	spb := make([]byte, 8)
	binary.LittleEndian.PutUint64(spb, 1)

	for i := 0; i < oneLevel; i++ {
		a := &pipeline.PipeWriteArgs{Ref: addr.Bytes(), Span: spb}
		ht.ChainWrite(a)
	}

	ref, err := ht.Sum()
	if err != nil {
		t.Fatal(err)
	}
	exphash := "281907c4199cd2b05b80469d2af5be103cc1317dcf78d5d9b95694aadb2d4994"
	if a := hex.EncodeToString(ref); a != exphash {
		t.Fatalf("expected hash %s but got %s", exphash, a)
	}
}
