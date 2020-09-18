// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import "github.com/ethersphere/bee/pkg/swarm"

type Server = server

type (
	BytesPostResponse        = bytesPostResponse
	FileUploadResponse       = fileUploadResponse
	TagResponse              = tagResponse
	TagRequest               = tagRequest
	PinnedChunk              = pinnedChunk
	ListPinnedChunksResponse = listPinnedChunksResponse
)

var (
	ContentTypeTar = contentTypeTar
)

var (
	ManifestRootPath                      = manifestRootPath
	ManifestWebsiteIndexDocumentSuffixKey = manifestWebsiteIndexDocumentSuffixKey
	ManifestWebsiteErrorDocumentPathKey   = manifestWebsiteErrorDocumentPathKey
)

var (
	ErrNoResolver           = errNoResolver
	ErrInvalidNameOrAddress = errInvalidNameOrAddress
)

func (s *Server) ResolveNameOrAddress(str string) (swarm.Address, error) {
	return s.resolveNameOrAddress(str)
}
