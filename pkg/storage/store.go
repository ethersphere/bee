package storage

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("storage: not found")

type Storer interface {
	Get(ctx context.Context, addr []byte) (data []byte, err error)
	Put(ctx context.Context, addr, data []byte) error
}
