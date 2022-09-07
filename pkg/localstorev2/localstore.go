package localstore

import (
	"context"
	"io"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

type PutterCloser interface {
	io.Closer
	storage.Putter
}

type PinStore interface {
	NewCollection(context.Context, swarm.Address) (PutterCloser, error)
	Pins(context.Context) ([]swarm.Address, error)
	HasPin(context.Context) (bool, error)
	DeletePin(context.Context, swarm.Address) error
}
