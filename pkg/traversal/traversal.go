// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package traversal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/ethersphere/bee/pkg/collection/entry"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Service is the service to find dependent chunks for an address.
type Service interface {
	// TraverseChunkAddresses iterates through each address related to
	// the supplied one, if possible.
	TraverseChunkAddresses(context.Context, swarm.Address, bool, swarm.AddressIterFunc) error
}

type traversalService struct {
	logger logging.Logger
	storer storage.Storer
}

func NewService(logger logging.Logger, storer storage.Storer) (Service, error) {
	return &traversalService{
		logger: logger,
		storer: storer,
	}, nil
}

func (s *traversalService) TraverseChunkAddresses(
	ctx context.Context,
	reference swarm.Address,
	encrypted bool,
	chunkAddressFunc swarm.AddressIterFunc,
) (err error) {

	isFile, e, metadata, err := s.checkIsFile(ctx, reference)
	if err != nil {
		return err
	}

	// reference address could be missrepresented as file when:
	// - content size is 64 bytes (or 128 for encrypted reference)
	// - second reference exists and is JSON (and not actually file metadata)

	if isFile {

		isManifest, m, err := s.checkIsManifest(ctx, reference, encrypted, e, metadata)
		if err != nil {
			return err
		}

		// reference address could be missrepresented as manifest when:
		// - file content type is actually on of manifest type (manually set)
		// - content was unmarshalled
		//
		// even though content could be unmarshaled in some case, iteration
		// through addresses will not be possible

		if isManifest {
			// process as manifest

			err = m.IterateAddresses(ctx, func(manifestNodeAddr swarm.Address) (stop bool) {
				err := s.TraverseChunkAddresses(ctx, manifestNodeAddr, encrypted, chunkAddressFunc)
				if err != nil {
					stop = true
				}
				return
			})
			if err != nil {
				return fmt.Errorf("traversal: iterate chunks: %s: %w", reference, err)
			}

			metadataReference := e.Metadata()

			err = s.processBytes(ctx, metadataReference, chunkAddressFunc)
			if err != nil {
				return nil
			}

			if stop := chunkAddressFunc(reference); stop {
				return nil
			}

		} else {
			// process as file

			bytesReference := e.Reference()

			err = s.processBytes(ctx, bytesReference, chunkAddressFunc)
			if err != nil {
				return nil
			}

			metadataReference := e.Metadata()

			err = s.processBytes(ctx, metadataReference, chunkAddressFunc)
			if err != nil {
				return nil
			}

			if stop := chunkAddressFunc(reference); stop {
				return nil
			}
		}

	} else {
		// process as bytes

		err = s.processBytes(ctx, reference, chunkAddressFunc)
		if err != nil {
			return nil
		}
	}

	return nil
}

// checkIsFile checks if the content is file.
func (s *traversalService) checkIsFile(
	ctx context.Context,
	reference swarm.Address,
) (isFile bool, e *entry.Entry, metadata *entry.Metadata, err error) {

	var (
		j    file.Joiner
		span int64
	)

	j, span, err = joiner.New(ctx, s.storer, reference)
	if err != nil {
		err = fmt.Errorf("traversal: joiner: %s: %w", reference, err)
		return
	}

	maybeIsFile := entry.CanUnmarshal(span)

	if maybeIsFile {
		buf := bytes.NewBuffer(nil)
		_, err = file.JoinReadAll(ctx, j, buf)
		if err != nil {
			err = fmt.Errorf("traversal: read entry: %s: %w", reference, err)
			return
		}

		e = &entry.Entry{}
		err = e.UnmarshalBinary(buf.Bytes())
		if err != nil {
			err = fmt.Errorf("traversal: unmarshal entry: %s: %w", reference, err)
			return
		}

		// NOTE: any bytes will unmarshall to addresses; we need to check metadata

		// read metadata
		j, _, err = joiner.New(ctx, s.storer, e.Metadata())
		if err != nil {
			// ignore
			err = nil
			return
		}

		buf = bytes.NewBuffer(nil)
		_, err = file.JoinReadAll(ctx, j, buf)
		if err != nil {
			err = fmt.Errorf("traversal: read metadata: %s: %w", reference, err)
			return
		}
		metadata = &entry.Metadata{}
		err = json.Unmarshal(buf.Bytes(), metadata)
		if err != nil {
			// may not be metadata JSON
			err = nil
			return
		}

		isFile = true
	}

	return
}

// checkIsManifest checks if the content is manifest.
func (s *traversalService) checkIsManifest(
	ctx context.Context,
	reference swarm.Address,
	encrypted bool,
	e *entry.Entry,
	metadata *entry.Metadata,
) (isManifest bool, m manifest.Interface, err error) {

	m, err = manifest.NewManifestReference(
		ctx,
		metadata.MimeType,
		e.Reference(),
		encrypted,
		s.storer,
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

	err = j.IterateChunkAddresses(func(addr swarm.Address) (stop bool) {
		stop = chunkAddressFunc(addr)
		return
	})
	if err != nil {
		return fmt.Errorf("traversal: iterate chunks: %s: %w", reference, err)
	}

	return nil
}
