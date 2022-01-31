// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ethersphere/bee/pkg/jsonhttp"
)

type auth interface {
	Enforce(string, string, string) (bool, error)
}

func PermissionCheckHandler(auth auth) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqToken := r.Header.Get("Authorization")
			if !strings.HasPrefix(reqToken, "Bearer ") {
				jsonhttp.Forbidden(w, "Missing bearer token")
				return
			}

			keys := strings.Split(reqToken, "Bearer ")

			if len(keys) != 2 || strings.Trim(keys[1], " ") == "" {
				jsonhttp.Unauthorized(w, "Missing security token")
				return
			}

			apiKey := keys[1]

			allowed, err := auth.Enforce(apiKey, r.URL.Path, r.Method)
			if errors.Is(err, ErrTokenExpired) {
				jsonhttp.Unauthorized(w, "Token expired")
				return
			}

			if err != nil {
				jsonhttp.InternalServerError(w, "Error occurred while validating the security token")
				return
			}

			if !allowed {
				jsonhttp.Forbidden(w, "Provided security token does not grant access to the resource")
				return
			}

			h.ServeHTTP(w, r)
		})
	}
}
