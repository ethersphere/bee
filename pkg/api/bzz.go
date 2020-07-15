// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/ethersphere/bee/pkg/collection/entry"
	"github.com/ethersphere/bee/pkg/encryption"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

const (
	ManifestType = "application/bzz-manifest+json"
)

func (s *server) bzzDownloadHandler(w http.ResponseWriter, r *http.Request) {
	addressHex := mux.Vars(r)["address"]
	path := mux.Vars(r)["path"]

	ctx := r.Context()

	address, err := swarm.ParseHexAddress(addressHex)
	if err != nil {
		s.Logger.Debugf("bzz download: parse address %s: %v", addressHex, err)
		s.Logger.Error("bzz download: parse address error")
		jsonhttp.BadRequest(w, "invalid address")
		return
	}

	toDecrypt := len(address.Bytes()) == (swarm.HashSize + encryption.KeyLength)

	// read manifest entry
	j := joiner.NewSimpleJoiner(s.Storer)
	buf := bytes.NewBuffer(nil)
	_, err = file.JoinReadAll(j, address, buf, toDecrypt)
	if err != nil {
		s.Logger.Debugf("bzz download: read entry %s: %v", address, err)
		s.Logger.Errorf("bzz download: read entry %s", address)
		jsonhttp.NotFound(w, nil)
		return
	}
	e := &entry.Entry{}
	err = e.UnmarshalBinary(buf.Bytes())
	if err != nil {
		s.Logger.Debugf("bzz download: unmarshal entry %s: %v", address, err)
		s.Logger.Errorf("bzz download: unmarshal entry %s", address)
		jsonhttp.InternalServerError(w, "error unmarshaling entry")
		return
	}

	// read metadata
	buf = bytes.NewBuffer(nil)
	_, err = file.JoinReadAll(j, e.Metadata(), buf, toDecrypt)
	if err != nil {
		s.Logger.Debugf("bzz download: read metadata %s: %v", address, err)
		s.Logger.Errorf("bzz download: read metadata %s", address)
		jsonhttp.NotFound(w, nil)
		return
	}
	metadata := &entry.Metadata{}
	err = json.Unmarshal(buf.Bytes(), metadata)
	if err != nil {
		s.Logger.Debugf("bzz download: unmarshal metadata %s: %v", address, err)
		s.Logger.Errorf("bzz download: unmarshal metadata %s", address)
		jsonhttp.InternalServerError(w, "error unmarshaling metadata")
		return
	}

	// we are expecting manifest Mime type here
	if ManifestType != metadata.MimeType {
		jsonhttp.BadRequest(w, "not manifest")
		return
	}

	// read manifest content
	buf = bytes.NewBuffer(nil)
	_, err = file.JoinReadAll(j, e.Reference(), buf, toDecrypt)
	if err != nil {
		s.Logger.Debugf("bzz download: data join %s: %v", address, err)
		s.Logger.Errorf("bzz download: data join %s", address)
		jsonhttp.NotFound(w, nil)
		return
	}
	manifest, err := s.ManifestParser.Parse(buf.Bytes())
	if err != nil {
		s.Logger.Debugf("bzz download: unmarshal manifest %s: %v", address, err)
		s.Logger.Errorf("bzz download: unmarshal manifest %s", address)
		jsonhttp.InternalServerError(w, "error unmarshaling manifest")
		return
	}

	entry, err := manifest.FindEntry(path)
	if err != nil {
		jsonhttp.BadRequest(w, "invalid path address")
		return
	}

	entryAddress := entry.GetAddress()

	toDecryptEntry := len(entryAddress.Bytes()) == (swarm.HashSize + encryption.KeyLength)

	dataSize, err := j.Size(ctx, entryAddress)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			s.Logger.Debugf("bzz download: not found %s: %v", entryAddress, err)
			s.Logger.Error("bzz download: not found")
			jsonhttp.NotFound(w, "not found")
			return
		}
		s.Logger.Debugf("bzz download: invalid root chunk %s: %v", entryAddress, err)
		s.Logger.Error("bzz download: invalid root chunk")
		jsonhttp.BadRequest(w, "invalid root chunk")
		return
	}

	outBuffer := bytes.NewBuffer(nil)
	c, err := file.JoinReadAll(j, entryAddress, outBuffer, toDecryptEntry)
	if err != nil && c == 0 {
		s.Logger.Debugf("bzz download: data join %s: %v", entryAddress, err)
		s.Logger.Errorf("bzz download: data join %s", entryAddress)
		jsonhttp.NotFound(w, nil)
		return
	}

	pr, pw := io.Pipe()
	defer pr.Close()
	go func() {
		ctx := r.Context()
		<-ctx.Done()
		if err := ctx.Err(); err != nil {
			if err := pr.CloseWithError(err); err != nil {
				s.Logger.Debugf("bzz download: data join close %s: %v", entryAddress, err)
				s.Logger.Errorf("bzz download: data join close %s", entryAddress)
			}
		}
	}()

	go func() {
		_, err := file.JoinReadAll(j, entryAddress, pw, toDecrypt)
		if err := pw.CloseWithError(err); err != nil {
			s.Logger.Debugf("bzz download: data join close %s: %v", entryAddress, err)
			s.Logger.Errorf("bzz download: data join close %s", entryAddress)
		}
	}()

	bpr := bufio.NewReader(pr)

	if b, err := bpr.Peek(4096); err != nil && err != io.EOF && len(b) == 0 {
		s.Logger.Debugf("bzz download: data join %s: %v", entryAddress, err)
		s.Logger.Errorf("bzz download: data join %s", entryAddress)
		jsonhttp.NotFound(w, nil)
		return
	}

	// include all headers from manifest
	for name, values := range entry.GetHeaders() {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	w.Header().Set("ETag", fmt.Sprintf("%q", entryAddress))
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", entry.GetName()))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", dataSize))
	w.Header().Set("Decompressed-Content-Length", fmt.Sprintf("%d", dataSize))
	if _, err = io.Copy(w, bpr); err != nil {
		s.Logger.Debugf("bzz download: data read %s: %v", entryAddress, err)
		s.Logger.Errorf("bzz download: data read %s", entryAddress)
	}
}
