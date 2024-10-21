// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonhttp

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type MethodHandler map[string]http.Handler

func (h MethodHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	HandleMethods(h, `{"message":"Method Not Allowed","code":405}`, DefaultContentTypeHeader, w, r)
}

// HandleMethods uses a corresponding Handler based on HTTP request method.
// If Handler is not found, a method not allowed HTTP response is returned
// with specified body and Content-Type header.
func HandleMethods(methods map[string]http.Handler, body string, contentType string, w http.ResponseWriter, r *http.Request) {
	if handler, ok := methods[r.Method]; ok {
		handler.ServeHTTP(w, r)
		return
	}

	allow := make([]string, 0, len(methods))
	for k := range methods {
		allow = append(allow, k)
	}
	sort.Strings(allow)

	// true if not COR request
	if len(w.Header().Get("Access-Control-Allow-Methods")) == 0 {
		w.Header().Set("Allow", strings.Join(allow, ", "))
	} else {
		w.Header().Set("Access-Control-Allow-Methods", strings.Join(allow, ", "))
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusMethodNotAllowed)
	fmt.Fprintln(w, body)
}

func NotFoundHandler(w http.ResponseWriter, _ *http.Request) {
	NotFound(w, nil)
}

// NewMaxBodyBytesHandler is an http middleware constructor that limits the
// maximal number of bytes that can be read from the request body. When a body
// is read, the error can be handled with a helper function HandleBodyReadError
// in order to respond with Request Entity Too Large response.
// See TestNewMaxBodyBytesHandler as an example.
func NewMaxBodyBytesHandler(limit int64) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > limit {
				RequestEntityTooLarge(w, nil)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			h.ServeHTTP(w, r)
		})
	}
}

// HandleBodyReadError checks for particular errors and writes appropriate
// response accordingly. If no known error is found, no response is written and
// the function returns false.
func HandleBodyReadError(err error, w http.ResponseWriter) (responded bool) {
	if err == nil {
		return false
	}
	// http.MaxBytesReader returns an unexported error,
	// this is the only way to detect it
	if err.Error() == "http: request body too large" {
		RequestEntityTooLarge(w, nil)
		return true
	}
	return false
}
