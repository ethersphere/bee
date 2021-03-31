// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package traversal provides abstraction and implementation
// needed to traverse all chunks below a given root hash.
// It tries to parse all manifests and collections in its
// attempt to log all chunk addresses on the way.
package traversal

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/file/loadsave"
	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	// ErrInvalidType is returned when the reference was not expected type.
	ErrInvalidType = errors.New("traversal: invalid type")
)

// Service is the service to find dependent chunks for an address.
type Service interface {
	// TraverseAddresses iterates through each address related to the supplied
	// one, if possible.
	TraverseAddresses(context.Context, swarm.Address, swarm.AddressIterFunc) error

	// TraverseBytesAddresses iterates through each address of a bytes.
	TraverseBytesAddresses(context.Context, swarm.Address, swarm.AddressIterFunc) error
	// TraverseManifestAddresses iterates through each address of a manifest,
	// as well as each entry found in it.
	TraverseManifestAddresses(context.Context, swarm.Address, swarm.AddressIterFunc) error
}

type traversalService struct {
	storer storage.Storer
}

func NewService(storer storage.Storer) Service {
	return &traversalService{
		storer: storer,
	}
}

func (s *traversalService) TraverseAddresses(
	ctx context.Context,
	reference swarm.Address,
	chunkAddressFunc swarm.AddressIterFunc,
) error {

	isManifest, m, err := s.checkIsManifest(ctx, reference)
	if err != nil {
		return err
	}

	if isManifest {
		return m.IterateAddresses(ctx, func(manifestNodeAddr swarm.Address) error {
			fmt.Println("Iterating", manifestNodeAddr.String())
			return s.processBytes(ctx, manifestNodeAddr, chunkAddressFunc)
		})
	}
	return s.processBytes(ctx, reference, chunkAddressFunc)
}

func (s *traversalService) TraverseBytesAddresses(
	ctx context.Context,
	reference swarm.Address,
	chunkAddressFunc swarm.AddressIterFunc,
) error {
	return s.processBytes(ctx, reference, chunkAddressFunc)
}

func (s *traversalService) TraverseManifestAddresses(
	ctx context.Context,
	reference swarm.Address,
	chunkAddressFunc swarm.AddressIterFunc,
) error {

	isManifest, m, err := s.checkIsManifest(ctx, reference)
	if err != nil {
		return err
	}
	if !isManifest {
		return ErrInvalidType
	}

	err = m.IterateAddresses(ctx, func(manifestNodeAddr swarm.Address) error {
		fmt.Println("Iterating", manifestNodeAddr.String())
		return s.processBytes(ctx, manifestNodeAddr, chunkAddressFunc)
	})
	if err != nil {
		return fmt.Errorf("traversal: iterate chunks: %s: %w", reference, err)
	}

	return nil
}

// checkIsManifest checks if the content is manifest.
func (s *traversalService) checkIsManifest(
	ctx context.Context,
	reference swarm.Address,
) (isManifest bool, m manifest.Interface, err error) {

	// NOTE: 'encrypted' parameter only used for saving manifest
	m, err = manifest.NewDefaultManifestReference(
		reference,
		loadsave.New(s.storer, storage.ModePutRequest, false),
	)
	if err != nil {
		if err == manifest.ErrInvalidManifestType {
			// ignore
			err = nil
			return
		}
		err = fmt.Errorf("traversal: read manifest: %s: %w", reference, err)
		return
	}
	fmt.Println("Found Manifest", m.Type())
	isManifest = true
	return
}

func (s *traversalService) processBytes(
	ctx context.Context,
	reference swarm.Address,
	chunkAddressFunc swarm.AddressIterFunc,
) error {
	j, _, err := joiner.New(ctx, s.storer, reference)
	if err != nil {
		return fmt.Errorf("traversal: joiner: %s: %w", reference, err)
	}

	err = j.IterateChunkAddresses(chunkAddressFunc)
	if err != nil {
		return fmt.Errorf("traversal: iterate chunks: %s: %w", reference, err)
	}

	return nil
}
