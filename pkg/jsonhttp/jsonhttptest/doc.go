// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package jsonhttptest helps with end-to-end testing of JSON-based HTTP APIs.

To test specific endpoint, Request function should be called with options that
should validate response from the server:

	options := []jsonhttptest.Option{
		jsonhttptest.WithRequestHeader("Content-Type", "text/html"),
	}
	jsonhttptest.Request(t, client, http.MethodGet, "/", http.StatusOk, options...)
	// ...

The HTTP request will be executed using the supplied client, and response
checked in expected status code is returned, as well as with each configured
option function.
*/
package jsonhttptest
