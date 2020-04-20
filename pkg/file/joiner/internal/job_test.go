package internal_test

import (
	"context"
	"testing"

	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/file/joiner/internal"
)

func TestSimpleJoinerJob(t *testing.T) {
	store := mock.NewStorer()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	j := internal.NewSimpleJoinerJob(ctx, store, 4096, []byte{})
	_ = j
}
