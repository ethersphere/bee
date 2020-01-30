// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonhttp

import (
	"net/http"

	"resenje.org/web"
)

type MethodHandler map[string]http.Handler

func (h MethodHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	web.HandleMethods(h, `{"message":"Method Not Allowed","code":405}`, DefaultContentTypeHeader, w, r)
}

func NotFoundHandler(w http.ResponseWriter, _ *http.Request) {
	NotFound(w, nil)
}
