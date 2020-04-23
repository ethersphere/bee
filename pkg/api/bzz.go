// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	bmtlegacy "github.com/ethersphere/bmt/legacy"
	"github.com/gorilla/mux"
	"golang.org/x/crypto/sha3"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

func hashFunc() hash.Hash {
	return sha3.NewLegacyKeccak256()
}

func (s *server) bzzUploadHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		s.Logger.Debugf("bzz: read error: %v", err)
		s.Logger.Error("bzz: read error")
		jsonhttp.InternalServerError(w, "cannot read request")
		return
	}

	p := bmtlegacy.NewTreePool(hashFunc, swarm.SectionSize, bmtlegacy.PoolSize)
	hasher := bmtlegacy.New(p)
	span := len(data)
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(span))
	data = append(b, data...)
	err = hasher.SetSpan(int64(span))
	if err != nil {
		s.Logger.Debugf("bzz: hasher set span: %v", err)
		s.Logger.Error("bzz: hash data error")
		jsonhttp.InternalServerError(w, "cannot hash data")
		return
	}
	_, err = hasher.Write(data[8:])
	if err != nil {
		s.Logger.Debugf("bzz: hasher write: %v", err)
		s.Logger.Error("bzz: hash data error")
		jsonhttp.InternalServerError(w, "cannot hash data")
		return
	}
	addr := swarm.NewAddress(hasher.Sum(nil))
	_, err = s.Storer.Put(ctx, storage.ModePutUpload, swarm.NewChunk(addr, data[8:]))
	if err != nil {
		s.Logger.Debugf("bzz: write error: %v, addr %s", err, addr)
		s.Logger.Error("bzz: write error")
		jsonhttp.InternalServerError(w, "write error")
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = io.Copy(w, strings.NewReader(addr.String()))
}

func (s *server) bzzGetHandler(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["address"]
	ctx := r.Context()

	address, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.Logger.Debugf("bzz: parse address %s: %v", addr, err)
		s.Logger.Error("bzz: parse address error")
		jsonhttp.BadRequest(w, "invalid address")
		return
	}

	chunk, err := s.Storer.Get(ctx, storage.ModeGetRequest, address)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			s.Logger.Trace("bzz: not found. addr %s", address)
			jsonhttp.NotFound(w, "not found")
			return

		}
		s.Logger.Debugf("bzz: read error: %v ,addr %s", err, address)
		s.Logger.Error("bzz: read error")
		jsonhttp.InternalServerError(w, "read error")
		return
	}
	w.Header().Set("Content-Type", "binary/octet-stream")
	_, _ = io.Copy(w, bytes.NewReader(chunk.Data()))
}
