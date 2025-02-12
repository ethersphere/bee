// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/p2p/mock"
)

func TestGetWelcomeMessage(t *testing.T) {
	t.Parallel()

	const DefaultTestWelcomeMessage = "Hello World!"

	srv, _, _, _ := newTestServer(t, testServerOptions{
		P2P: mock.New(mock.WithGetWelcomeMessageFunc(func() string {
			return DefaultTestWelcomeMessage
		}))})

	jsonhttptest.Request(t, srv, http.MethodGet, "/welcome-message", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(api.WelcomeMessageResponse{
			WelcomeMesssage: DefaultTestWelcomeMessage,
		}),
	)
}

// nolint:paralleltest
func TestSetWelcomeMessage(t *testing.T) {
	testCases := []struct {
		desc        string
		message     string
		wantFail    bool
		wantStatus  int
		wantMessage string
	}{
		{
			desc:       "OK",
			message:    "Changed value",
			wantStatus: http.StatusOK,
		},
		{
			desc:       "OK - welcome message empty",
			message:    "",
			wantStatus: http.StatusOK,
		},
		{
			desc:     "error - request entity too large",
			wantFail: true,
			message: `zZZbzbzbzbBzBBZbBbZbbbBzzzZBZBbzzBBBbBzBzzZbbBzBBzBBbZz
			bZZZBBbbZbbZzBbzBbzbZBZzBZZbZzZzZzbbZZBZzzbBZBzZzzBBzZZzzZbZZZzbbbzz
			bBzZZBbBZBzZzBZBzbzBBbzBBzbzzzBbBbZzZBZBZzBZZbbZZBZZBzZzBZbzZBzZbBzZ
			bbbBbbZzZbzbZzZzbzzzbzzbzZZzbbzbBZZbBbBZBBZzZzzbBBBBBZbZzBzzBbzBbbbz
			BBzbbZBbzbzBZbzzBzbZBzzbzbbbBZBzBZzBZbzBzZzBZZZBzZZBzBZZzbzZbzzZzBBz
			ZZzbZzzZZZBZBBbZZbZzBBBzbzZZbbZZBZZBBBbBZzZbZBZBBBzzZBbbbbzBzbbzBBBz
			bZBBbZzBbZZBzbBbZZBzBzBzBBbzzzZBbzbZBbzBbZzbbBZBBbbZbBBbbBZbzbZzbBzB
			bBbbZZbzZzbbBbzZbZZZZbzzZZbBzZZbZzZzzBzbZZ`, // 513 characters
			wantStatus: http.StatusRequestEntityTooLarge,
		},
	}
	testURL := "/welcome-message"

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			mockP2P := mock.New()

			srv, _, _, _ := newTestServer(t, testServerOptions{
				P2P: mockP2P,
			})

			if tC.wantMessage == "" {
				tC.wantMessage = http.StatusText(tC.wantStatus)
			}
			data, _ := json.Marshal(api.WelcomeMessageRequest{
				WelcomeMesssage: tC.message,
			})
			body := bytes.NewReader(data)
			wantResponse := jsonhttp.StatusResponse{
				Message: tC.wantMessage,
				Code:    tC.wantStatus,
			}
			jsonhttptest.Request(t, srv, http.MethodPost, testURL, tC.wantStatus,
				jsonhttptest.WithRequestBody(body),
				jsonhttptest.WithExpectedJSONResponse(wantResponse),
			)
			if !tC.wantFail {
				got := mockP2P.GetWelcomeMessage()
				if got != tC.message {
					t.Fatalf("could not set dynamic welcome message: want %s, got %s", tC.message, got)
				}
			}
		})
	}
}

func TestSetWelcomeMessageInternalServerError(t *testing.T) {
	t.Parallel()

	testMessage := "NO CHANCE BYE"
	testError := errors.New("Could not set value")
	testURL := "/welcome-message"

	srv, _, _, _ := newTestServer(t, testServerOptions{
		P2P: mock.New(mock.WithSetWelcomeMessageFunc(func(string) error {
			return testError
		})),
	})

	data, _ := json.Marshal(api.WelcomeMessageRequest{
		WelcomeMesssage: testMessage,
	})
	body := bytes.NewReader(data)

	t.Run("internal server error - error on store", func(t *testing.T) {
		t.Parallel()

		wantCode := http.StatusInternalServerError
		wantResp := jsonhttp.StatusResponse{
			Message: testError.Error(),
			Code:    wantCode,
		}
		jsonhttptest.Request(t, srv, http.MethodPost, testURL, wantCode,
			jsonhttptest.WithRequestBody(body),
			jsonhttptest.WithExpectedJSONResponse(wantResp),
		)
	})
}
