// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/google/go-cmp/cmp"
)

// nolint:paralleltest
func TestGetLoggers(t *testing.T) {
	defer func(fn api.LogRegistryIterateFn) {
		api.ReplaceLogRegistryIterateFn(fn)
	}(api.LogRegistryIterate)

	fn := func(fn func(id, path string, verbosity log.Level, v uint) bool) {
		data := []struct {
			id        string
			path      string
			verbosity log.Level
			v         uint
		}{{
			id:        `one[0][]>>824634860360`,
			path:      "one",
			verbosity: log.VerbosityAll,
			v:         0,
		}, {
			id:        `one/name[0][]>>824634860360`,
			path:      "one/name",
			verbosity: log.VerbosityWarning,
			v:         0,
		}, {
			id:        `one/name[0][\"val\"=1]>>824634860360`,
			path:      "one/name",
			verbosity: log.VerbosityWarning,
			v:         0,
		}, {
			id:        `one/name[1][]>>824634860360`,
			path:      "one/name",
			verbosity: log.VerbosityInfo,
			v:         1,
		}, {
			id:        `one/name[2][]>>824634860360`,
			path:      "one/name",
			verbosity: log.VerbosityInfo,
			v:         2,
		}}

		for _, d := range data {
			if !fn(d.id, d.path, d.verbosity, d.v) {
				return
			}
		}
	}
	api.ReplaceLogRegistryIterateFn(fn)

	have := make(map[string]interface{})
	want := make(map[string]interface{})
	data := `{"loggers":[{"id":"b25lWzBdW10-PjgyNDYzNDg2MDM2MA==","logger":"one","subsystem":"one[0][]\u003e\u003e824634860360","verbosity":"all"},{"id":"b25lL25hbWVbMF1bXT4-ODI0NjM0ODYwMzYw","logger":"one/name","subsystem":"one/name[0][]\u003e\u003e824634860360","verbosity":"warning"},{"id":"b25lL25hbWVbMF1bXCJ2YWxcIj0xXT4-ODI0NjM0ODYwMzYw","logger":"one/name","subsystem":"one/name[0][\\\"val\\\"=1]\u003e\u003e824634860360","verbosity":"warning"},{"id":"b25lL25hbWVbMV1bXT4-ODI0NjM0ODYwMzYw","logger":"one/name","subsystem":"one/name[1][]\u003e\u003e824634860360","verbosity":"info"},{"id":"b25lL25hbWVbMl1bXT4-ODI0NjM0ODYwMzYw","logger":"one/name","subsystem":"one/name[2][]\u003e\u003e824634860360","verbosity":"info"}],"tree":{"one":{"+":["all|one[0][]\u003e\u003e824634860360"],"/":{"name":{"+":["warning|one/name[0][]\u003e\u003e824634860360","warning|one/name[0][\\\"val\\\"=1]\u003e\u003e824634860360","info|one/name[1][]\u003e\u003e824634860360","info|one/name[2][]\u003e\u003e824634860360"]}}}}}`
	if err := json.Unmarshal([]byte(data), &want); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	client, _, _, _ := newTestServer(t, testServerOptions{})
	jsonhttptest.Request(t, client, http.MethodGet, "/loggers", http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&have),
	)

	if diff := cmp.Diff(want, have); diff != "" {
		t.Errorf("response body mismatch (-want +have):\n%s", diff)
	}
}

// nolint:paralleltest
func TestSetLoggerVerbosity(t *testing.T) {
	defer func(fn api.LogSetVerbosityByExpFn) {
		api.ReplaceLogSetVerbosityByExp(fn)
	}(api.LogSetVerbosityByExp)

	client, _, _, _ := newTestServer(t, testServerOptions{})

	type data struct {
		exp string
		ver log.Level
	}

	have := new(data)
	api.ReplaceLogSetVerbosityByExp(func(e string, v log.Level) error {
		have.exp = e
		have.ver = v
		return nil
	})

	tests := []struct {
		want data
	}{{
		want: data{exp: `name`, ver: log.VerbosityWarning},
	}, {
		want: data{exp: `^name$`, ver: log.VerbosityWarning},
	}, {
		want: data{exp: `^name/\[0`, ver: log.VerbosityWarning},
	}, {
		want: data{exp: `^name/\[0\]\[`, ver: log.VerbosityWarning},
	}, {
		want: data{exp: `^name/\[0\]\[\"val\"=1\]`, ver: log.VerbosityWarning},
	}, {
		want: data{exp: `^name/\[0\]\[\"val\"=1\]>>824634860360`, ver: log.VerbosityWarning},
	}}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("to=%s,exp=%s", tc.want.ver, tc.want.exp), func(t *testing.T) {
			exp := base64.URLEncoding.EncodeToString([]byte(tc.want.exp))
			url := fmt.Sprintf("/loggers/%s/%s", exp, tc.want.ver)
			jsonhttptest.Request(t, client, http.MethodPut, url, http.StatusOK,
				jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
					Message: http.StatusText(http.StatusOK),
					Code:    http.StatusOK,
				}),
			)

			if have.exp != tc.want.exp {
				t.Errorf("exp missmatch: want: %q; have: %q", tc.want.exp, have.exp)
			}

			if have.ver != tc.want.ver {
				t.Errorf("verbosity missmatch: want: %q; have: %q", tc.want.ver, have.ver)
			}
		})
	}
}

func Test_loggerGetHandler_invalidInputs(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{})

	tests := []struct {
		name string
		exp  string
		want jsonhttp.StatusResponse
	}{{
		name: "exp - illegal base64",
		exp:  "123",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "exp",
					Error: "illegal base64 data at input byte 0",
				},
			},
		},
	}, {
		name: "exp - invalid regex",
		exp:  base64.URLEncoding.EncodeToString([]byte("[")),
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "exp",
					Error: "error parsing regexp: missing closing ]: `[`",
				},
			},
		},
	}}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			jsonhttptest.Request(t, client, http.MethodGet, "/loggers/"+tc.exp, tc.want.Code,
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}

func Test_loggerSetVerbosityHandler_invalidInputs(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{})

	tests := []struct {
		name      string
		exp       string
		verbosity string
		want      jsonhttp.StatusResponse
	}{{
		name:      "exp - illegal base64",
		exp:       "123",
		verbosity: "info",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "exp",
					Error: "illegal base64 data at input byte 0",
				},
			},
		},
	}, {
		name:      "exp - invalid regex",
		exp:       base64.URLEncoding.EncodeToString([]byte("[")),
		verbosity: "info",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "exp",
					Error: "error parsing regexp: missing closing ]: `[`",
				},
			},
		},
	}, {
		name:      "verbosity - invalid value",
		exp:       base64.URLEncoding.EncodeToString([]byte("123")),
		verbosity: "invalid",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "verbosity",
					Error: "want oneof:none error warning info debug all",
				},
			},
		},
	}}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			jsonhttptest.Request(t, client, http.MethodPut, "/loggers/"+tc.exp+"/"+tc.verbosity, tc.want.Code,
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}
