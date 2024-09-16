// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nbhdutil_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/util/nbhdutil"
)

type mockHttpClient struct {
	res string
}

func (c *mockHttpClient) Get(_ string) (*http.Response, error) {
	return &http.Response{
		Body: io.NopCloser(strings.NewReader(c.res)),
	}, nil
}

func Test_FetchNeighborhood(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		neighborhood     string
		suggester        string
		httpRes          string
		wantNeighborhood string
		wantError        bool
	}{
		{
			name:             "no suggester",
			suggester:        "",
			wantNeighborhood: "",
			wantError:        false,
		},
		{
			name:             "invalid suggester url",
			suggester:        "abc",
			wantNeighborhood: "",
			wantError:        true,
		},
		{
			name:             "missing neighborhood in res",
			suggester:        "http://test.com",
			wantNeighborhood: "",
			wantError:        false,
			httpRes:          `{"abc":"abc"}`,
		},
		{
			name:             "invalid neighborhood",
			suggester:        "http://test.com",
			wantNeighborhood: "",
			wantError:        true,
			httpRes:          `{"neighborhood":"abc"}`,
		},
		{
			name:             "valid neighborhood",
			suggester:        "http://test.com",
			wantNeighborhood: "11011101000",
			wantError:        false,
			httpRes:          `{"neighborhood":"11011101000"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			neighborhood, err := nbhdutil.FetchNeighborhood(&mockHttpClient{res: test.httpRes}, test.suggester)
			if test.wantNeighborhood != neighborhood {
				t.Fatalf("invalid neighborhood. want %s, got %s", test.wantNeighborhood, neighborhood)
			}
			if test.wantError && err == nil {
				t.Fatalf("expected error. got no error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("expected no error. got %v", err)
			}
		})
	}
}
