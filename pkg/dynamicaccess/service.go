package dynamicaccess

import (
	"context"
	"crypto/ecdsa"
	"io"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type Service interface {
	DownloadHandler(ctx context.Context, timestamp int64, encryptedRef swarm.Address, publisher *ecdsa.PublicKey, historyRootHash swarm.Address) (swarm.Address, error)
	UploadHandler(ctx context.Context, reference swarm.Address, publisher *ecdsa.PublicKey, historyRootHash *swarm.Address) (swarm.Address, swarm.Address, swarm.Address, error)
	io.Closer
}

// TODO: is service needed at all? -> it is just a wrapper around controller
type service struct {
	controller Controller
}

func (s *service) DownloadHandler(ctx context.Context, timestamp int64, encryptedRef swarm.Address, publisher *ecdsa.PublicKey, historyRootHash swarm.Address) (swarm.Address, error) {
	return s.controller.DownloadHandler(ctx, timestamp, encryptedRef, publisher, historyRootHash)
}

func (s *service) UploadHandler(ctx context.Context, reference swarm.Address, publisher *ecdsa.PublicKey, historyRootHash *swarm.Address) (swarm.Address, swarm.Address, swarm.Address, error) {
	return s.controller.UploadHandler(ctx, reference, publisher, historyRootHash)
}

// TODO: what to do in close ?
func (s *service) Close() error {
	return nil
}

func NewService(controller Controller) (Service, error) {
	return &service{
		controller: controller,
	}, nil
}
