// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ethersphere/bee/pkg/collection/entry"
	"github.com/ethersphere/bee/pkg/encryption"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

const (
	// ManifestContentType represents content type used for noting that specific
	// file should be processed as manifest
	ManifestContentType = "application/bzz-manifest+json"
)

func (s *server) bzzDownloadHandler(w http.ResponseWriter, r *http.Request) {
	addressHex := mux.Vars(r)["address"]
	path := mux.Vars(r)["path"]

	address, err := swarm.ParseHexAddress(addressHex)
	if err != nil {
		s.Logger.Debugf("bzz download: parse address %s: %v", addressHex, err)
		s.Logger.Error("bzz download: parse address")
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
	if ManifestContentType != metadata.MimeType {
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

	entryAddress := entry.GetReference()

	additionalHeaders := http.Header{}

	// copy headers from manifest
	if entry.GetHeaders() != nil {
		for name, values := range entry.GetHeaders() {
			for _, value := range values {
				additionalHeaders.Add(name, value)
			}
		}
	}

	// include filename
	if entry.GetName() != "" {
		additionalHeaders.Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", entry.GetName()))
	}

	s.downloadHandler(w, r, entryAddress, additionalHeaders)
}
