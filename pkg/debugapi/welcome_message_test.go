// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"bytes"
	"errors"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/p2p/mock"
)

func TestGetWelcomeMessage(t *testing.T) {
	const DefaultTestWelcomeMessage = "Hello World!"

	srv := newTestServer(t, testServerOptions{
		P2P: mock.New(mock.WithGetWelcomeMessageFunc(func() string {
			return DefaultTestWelcomeMessage
		}))})

	jsonhttptest.ResponseDirect(t, srv.Client, http.MethodGet, "/welcome-message", nil, http.StatusOK, debugapi.WelcomeMessageResponse{
		WelcomeMesssage: DefaultTestWelcomeMessage,
	})
}

func TestSetWelcomeMessageTDT(t *testing.T) {
	testCases := []struct {
		desc         string
		message      string
		wantStatus   int
		wantResponse interface{}
		wantFail     bool
	}{
		{
			desc:       "OK",
			message:    "Changed value",
			wantStatus: http.StatusOK,
			wantResponse: jsonhttp.StatusResponse{
				Message: "OK",
				Code:    http.StatusOK,
			},
		},
		{
			desc:       "OK - welcome message empty",
			message:    "",
			wantStatus: http.StatusOK,
			wantResponse: jsonhttp.StatusResponse{
				Message: "OK",
				Code:    http.StatusOK,
			},
		},
		{
			desc: "bad request - welcome message too long",
			message: `Lorem ipsum dolor sit amet, consectetur adipiscing elit.
			Maecenas eu aliquam enim. Nulla tincidunt arcu nec nulla condimentum nullam sodales`, // 141 characters
			wantStatus: http.StatusBadRequest,
			wantResponse: jsonhttp.StatusResponse{
				Message: "welcome message length exceeds maximum of 140",
				Code:    http.StatusBadRequest,
			},
			wantFail: true,
		},
	}

	srv := newTestServer(t, testServerOptions{
		P2P: mock.New(),
	})

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			jsonhttptest.ResponseDirect(t, srv.Client, http.MethodPost, "/welcome-message", bytes.NewReader([]byte(tC.message)), tC.wantStatus, tC.wantResponse)
			if !tC.wantFail {
				got := srv.P2PMock.GetWelcomeMessage()
				if got != tC.message {
					t.Fatalf("could not set dynamic welcome message: want %s, got %s", tC.message, got)
				}
			}
		})
	}
}

func TestSetWelcomeMessageInternalFail(t *testing.T) {
	testMessage := "NO CHANCE BYE"
	testError := errors.New("Could not set value")

	srv := newTestServer(t, testServerOptions{
		P2P: mock.New(mock.WithSetWelcomeMessageFunc(func(string) error {
			return testError
		})),
	})

	t.Run("internal server error - failed to store", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, srv.Client, http.MethodPost, "/welcome-message", bytes.NewReader([]byte(testMessage)), http.StatusInternalServerError, jsonhttp.StatusResponse{
			Message: testError.Error(),
			Code:    http.StatusInternalServerError,
		})
	})

}
