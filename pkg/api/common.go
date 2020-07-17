// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/ethersphere/bee/pkg/encryption"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	defaultBufSize = 4096
)

// downloadHandler contains common logic for dowloading Swarm file from API
func (s *server) internalDownloadHandler(
	w http.ResponseWriter,
	r *http.Request,
	reference swarm.Address,
	additionalHeaders http.Header,
) {
	ctx := r.Context()

	toDecrypt := len(reference.Bytes()) == (swarm.HashSize + encryption.KeyLength)

	j := joiner.NewSimpleJoiner(s.Storer)

	// send the file data back in the response
	dataSize, err := j.Size(ctx, reference)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			s.Logger.Debugf("api download: not found %s: %v", reference, err)
			s.Logger.Error("api download: not found")
			jsonhttp.NotFound(w, "not found")
			return
		}
		s.Logger.Debugf("api download: invalid root chunk %s: %v", reference, err)
		s.Logger.Error("api download: invalid root chunk")
		jsonhttp.BadRequest(w, "invalid root chunk")
		return
	}

	pr, pw := io.Pipe()
	defer pr.Close()
	go func() {
		ctx := r.Context()
		<-ctx.Done()
		if err := ctx.Err(); err != nil {
			if err := pr.CloseWithError(err); err != nil {
				s.Logger.Debugf("api download: data join close %s: %v", reference, err)
				s.Logger.Errorf("api download: data join close %s", reference)
			}
		}
	}()

	go func() {
		_, err := file.JoinReadAll(j, reference, pw, toDecrypt)
		if err := pw.CloseWithError(err); err != nil {
			s.Logger.Debugf("api download: data join close %s: %v", reference, err)
			s.Logger.Errorf("api download: data join close %s", reference)
		}
	}()

	bpr := bufio.NewReader(pr)

	if b, err := bpr.Peek(defaultBufSize); err != nil && err != io.EOF && len(b) == 0 {
		s.Logger.Debugf("api download: data join %s: %v", reference, err)
		s.Logger.Errorf("api download: data join %s", reference)
		jsonhttp.NotFound(w, nil)
		return
	}

	// include additional headers
	for name, values := range additionalHeaders {
		var v string
		for _, value := range values {
			if v != "" {
				v += "; "
			}
			v += value
		}
		w.Header().Set(name, v)
	}

	w.Header().Set("ETag", fmt.Sprintf("%q", reference))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", dataSize))
	w.Header().Set("Decompressed-Content-Length", fmt.Sprintf("%d", dataSize))
	if _, err = io.Copy(w, bpr); err != nil {
		s.Logger.Debugf("api download: data read %s: %v", reference, err)
		s.Logger.Errorf("api download: data read %s", reference)
	}
}
