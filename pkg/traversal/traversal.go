// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package traversal provides abstraction and implementation
// needed to traverse all chunks below a given root hash.
// It tries to parse all manifests and collections in its
// attempt to log all chunk addresses on the way.
package traversal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/collection/entry"
	"github.com/ethersphere/bee/pkg/file"
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
	// TraverseFileAddresses iterates through each address of a file.
	TraverseFileAddresses(context.Context, swarm.Address, swarm.AddressIterFunc) error
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

	isFile, e, metadata, err := s.checkIsFile(ctx, reference)
	if err != nil {
		return err
	}

	// reference address could be missrepresented as file when:
	// - content size is 64 bytes (or 128 for encrypted reference)
	// - second reference exists and is JSON (and not actually file metadata)

	if isFile {

		isManifest, m, err := s.checkIsManifest(ctx, reference, e, metadata)
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

			err = m.IterateAddresses(ctx, func(manifestNodeAddr swarm.Address) error {
				return s.traverseChunkAddressesFromManifest(ctx, manifestNodeAddr, chunkAddressFunc)
			})
			if err != nil {
				return fmt.Errorf("traversal: iterate chunks: %s: %w", reference, err)
			}

			metadataReference := e.Metadata()

			err = s.processBytes(ctx, metadataReference, chunkAddressFunc)
			if err != nil {
				return err
			}

			_ = chunkAddressFunc(reference)

		} else {
			return s.traverseChunkAddressesAsFile(ctx, reference, chunkAddressFunc, e)
		}

	} else {
		return s.processBytes(ctx, reference, chunkAddressFunc)
	}

	return nil
}

func (s *traversalService) TraverseBytesAddresses(
	ctx context.Context,
	reference swarm.Address,
	chunkAddressFunc swarm.AddressIterFunc,
) error {
	return s.processBytes(ctx, reference, chunkAddressFunc)
}

func (s *traversalService) TraverseFileAddresses(
	ctx context.Context,
	reference swarm.Address,
	chunkAddressFunc swarm.AddressIterFunc,
) error {

	isFile, e, _, err := s.checkIsFile(ctx, reference)
	if err != nil {
		return err
	}

	// reference address could be missrepresented as file when:
	// - content size is 64 bytes (or 128 for encrypted reference)
	// - second reference exists and is JSON (and not actually file metadata)

	if !isFile {
		return ErrInvalidType
	}

	return s.traverseChunkAddressesAsFile(ctx, reference, chunkAddressFunc, e)
}

func (s *traversalService) TraverseManifestAddresses(
	ctx context.Context,
	reference swarm.Address,
	chunkAddressFunc swarm.AddressIterFunc,
) error {

	isFile, e, metadata, err := s.checkIsFile(ctx, reference)
	if err != nil {
		return err
	}

	if !isFile {
		return ErrInvalidType
	}

	isManifest, m, err := s.checkIsManifest(ctx, reference, e, metadata)
	if err != nil {
		return err
	}

	// reference address could be missrepresented as manifest when:
	// - file content type is actually on of manifest type (manually set)
	// - content was unmarshalled
	//
	// even though content could be unmarshaled in some case, iteration
	// through addresses will not be possible

	if !isManifest {
		return ErrInvalidType
	}

	err = m.IterateAddresses(ctx, func(manifestNodeAddr swarm.Address) error {
		return s.traverseChunkAddressesFromManifest(ctx, manifestNodeAddr, chunkAddressFunc)
	})
	if err != nil {
		return fmt.Errorf("traversal: iterate chunks: %s: %w", reference, err)
	}

	metadataReference := e.Metadata()

	err = s.processBytes(ctx, metadataReference, chunkAddressFunc)
	if err != nil {
		return err
	}

	_ = chunkAddressFunc(reference)

	return nil
}

func (s *traversalService) traverseChunkAddressesFromManifest(
	ctx context.Context,
	reference swarm.Address,
	chunkAddressFunc swarm.AddressIterFunc,
) error {

	isFile, e, _, err := s.checkIsFile(ctx, reference)
	if err != nil {
		return err
	}

	if isFile {
		return s.traverseChunkAddressesAsFile(ctx, reference, chunkAddressFunc, e)
	}

	return s.processBytes(ctx, reference, chunkAddressFunc)
}

func (s *traversalService) traverseChunkAddressesAsFile(
	ctx context.Context,
	reference swarm.Address,
	chunkAddressFunc swarm.AddressIterFunc,
	e *entry.Entry,
) (err error) {

	bytesReference := e.Reference()

	err = s.processBytes(ctx, bytesReference, chunkAddressFunc)
	if err != nil {
		// possible it was custom JSON bytes, which matches entry JSON
		// but in fact is not file, and does not contain reference to
		// existing address, which is why it was not found in storage
		if !errors.Is(err, storage.ErrNotFound) {
			return nil
		}
		// ignore
	}

	metadataReference := e.Metadata()

	err = s.processBytes(ctx, metadataReference, chunkAddressFunc)
	if err != nil {
		return
	}

	_ = chunkAddressFunc(reference)

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

		// address sizes must match
		if len(reference.Bytes()) != len(e.Reference().Bytes()) {
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

		dec := json.NewDecoder(buf)
		dec.DisallowUnknownFields()
		err = dec.Decode(metadata)
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
	e *entry.Entry,
	metadata *entry.Metadata,
) (isManifest bool, m manifest.Interface, err error) {

	// NOTE: 'encrypted' parameter only used for saving manifest
	m, err = manifest.NewManifestReference(
		metadata.MimeType,
		e.Reference(),
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
