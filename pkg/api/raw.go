// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
)

type rawPostResponse struct {
	Hash swarm.Address `json:"hash"`
}
//func (s *server) bzzUploadHandler(w http.ResponseWriter, r *http.Request) {
//       ctx := r.Context()
//       data, err := ioutil.ReadAll(r.Body)
//       if err != nil {
//               s.Logger.Debugf("bzz: read error: %v", err)
//               s.Logger.Error("bzz: read error")
//               jsonhttp.InternalServerError(w, "cannot read request")
//               return
//       }
//
//       p := bmtlegacy.NewTreePool(hashFunc, swarm.Branches, bmtlegacy.PoolSize)
//       hasher := bmtlegacy.New(p)
//       span := len(data)
//       b := make([]byte, 8)
//       binary.LittleEndian.PutUint64(b, uint64(span))
//       data = append(b, data...)
//       err = hasher.SetSpan(int64(span))
//       if err != nil {
//               s.Logger.Debugf("bzz: hasher set span: %v", err)
//               s.Logger.Error("bzz: hash data error")
//               jsonhttp.InternalServerError(w, "cannot hash data")
//               return
//       }
//       _, err = hasher.Write(data[8:])
//       if err != nil {
//               s.Logger.Debugf("bzz: hasher write: %v", err)
//               s.Logger.Error("bzz: hash data error")
//               jsonhttp.InternalServerError(w, "cannot hash data")
//               return
//       }
//       addr := swarm.NewAddress(hasher.Sum(nil))
//       _, err = s.Storer.Put(ctx, storage.ModePutUpload, swarm.NewChunk(addr, data))
//       if err != nil {
//               s.Logger.Debugf("bzz: write error: %v, addr %s", err, addr)
//               s.Logger.Error("bzz: write error")
//               jsonhttp.InternalServerError(w, "write error")
//               return
//       }
//       jsonhttp.OK(w, bzzPostResponse{Hash: addr})
//}

func (s *server) rawUploadHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

       data, err := ioutil.ReadAll(r.Body)
       if err != nil {
               s.Logger.Debugf("raw: read error: %v", err)
	       s.Logger.Error("raw: read error")
               jsonhttp.InternalServerError(w, "cannot read request")
               return
       }

	s.Logger.Errorf("Content length %d", r.ContentLength)
	sp := splitter.NewSimpleSplitter(s.Storer)
	bodyReader := bytes.NewReader(data)
	bodyReadCloser := ioutil.NopCloser(bodyReader)
	address, err := sp.Split(ctx, bodyReadCloser, r.ContentLength)
	if err != nil {
		s.Logger.Debugf("raw: split error %s: %v", address, err)
		s.Logger.Error("raw: split error")
		jsonhttp.BadRequest(w, "split error")
		return
	}

	addressReader := bytes.NewReader(address.Bytes())
	_, _ = io.Copy(w, addressReader)
}

func (s *server) rawGetHandler(w http.ResponseWriter, r *http.Request) {
	addressHex := mux.Vars(r)["address"]
	ctx := r.Context()

	address, err := swarm.ParseHexAddress(addressHex)
	if err != nil {
		s.Logger.Debugf("raw: parse address %s: %v", addressHex, err)
		s.Logger.Error("raw: parse address error")
		jsonhttp.BadRequest(w, "invalid address")
		return
	}

	j := joiner.NewSimpleJoiner(s.Storer)

	dataSize, err := j.Size(ctx, address)
	if err != nil {
		s.Logger.Debugf("raw: invalid root chunk %s: %v", address, err)
		s.Logger.Error("raw: invalid root chunk")
		jsonhttp.BadRequest(w, "invalid root chunk")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", dataSize))
	err = file.JoinReadAll(j, address, w)
	if err != nil {
		s.Logger.Errorf("raw: data write %s: %v", address, err)
		s.Logger.Error("raw: data input error")
		jsonhttp.BadRequest(w, "failed to retrieve data")
		return
	}
}
