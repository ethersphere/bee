// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
)

func TestEndpointOptions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		serverOptions    testServerOptions
		expectedStatuses []struct {
			route           string
			expectedMethods []string
			expectedStatus  int
		}
	}{
		{
			name: "full_api_enabled",
			serverOptions: testServerOptions{
				FullAPIDisabled:    false,
				SwapDisabled:       false,
				ChequebookDisabled: false,
			},
			expectedStatuses: []struct {
				route           string
				expectedMethods []string
				expectedStatus  int
			}{
				// routes from mountTechnicalDebug
				{"/node", []string{"GET"}, http.StatusNoContent},
				{"/addresses", []string{"GET"}, http.StatusNoContent},
				{"/chainstate", []string{"GET"}, http.StatusNoContent},
				{"/debugstore", []string{"GET"}, http.StatusNoContent},
				{"/loggers", []string{"GET"}, http.StatusNoContent},
				{"/loggers/some-exp", []string{"GET"}, http.StatusNoContent},
				{"/loggers/some-exp/1", []string{"PUT"}, http.StatusNoContent},
				{"/readiness", nil, http.StatusBadRequest},
				{"/health", nil, http.StatusOK},
				{"/metrics", nil, http.StatusOK},
				{"/not_found", nil, http.StatusNotFound},

				// routes from mountAPI
				{"/", nil, http.StatusOK},
				{"/robots.txt", nil, http.StatusOK},
				{"/bytes", []string{"POST"}, http.StatusNoContent},
				{"/bytes/{address}", []string{"GET", "HEAD"}, http.StatusNoContent},
				{"/chunks", []string{"POST"}, http.StatusNoContent},
				{"/chunks/stream", nil, http.StatusBadRequest},
				{"/chunks/{address}", []string{"GET", "HEAD"}, http.StatusNoContent},
				{"/envelope/{address}", []string{"POST"}, http.StatusNoContent},
				{"/soc/{owner}/{id}", []string{"GET", "POST"}, http.StatusNoContent},
				{"/feeds/{owner}/{topic}", []string{"GET", "POST"}, http.StatusNoContent},
				{"/bzz", []string{"POST"}, http.StatusNoContent},
				{"/grantee", []string{"POST"}, http.StatusNoContent},
				{"/grantee/{address}", []string{"GET", "PATCH"}, http.StatusNoContent},
				{"/bzz/{address}", []string{"GET"}, http.StatusNoContent},
				{"/bzz/{address}/{path:.*}", []string{"GET", "HEAD"}, http.StatusNoContent},
				{"/pss/send/{topic}/{targets}", []string{"POST"}, http.StatusNoContent},
				{"/pss/subscribe/{topic}", nil, http.StatusBadRequest},
				{"/tags", []string{"GET", "POST"}, http.StatusNoContent},
				{"/tags/{id}", []string{"GET", "DELETE", "PATCH"}, http.StatusNoContent},
				{"/pins", []string{"GET"}, http.StatusNoContent},
				{"/pins/check", []string{"GET"}, http.StatusNoContent},
				{"/pins/{reference}", []string{"GET", "POST", "DELETE"}, http.StatusNoContent},
				{"/stewardship/{address}", []string{"GET", "PUT"}, http.StatusNoContent},

				// routes from mountBusinessDebug
				{"/transactions", []string{"GET"}, http.StatusNoContent},
				{"/transactions/{hash}", []string{"GET", "POST", "DELETE"}, http.StatusNoContent},
				{"/peers", []string{"GET"}, http.StatusNoContent},
				{"/pingpong/{address}", []string{"POST"}, http.StatusNoContent},
				{"/reservestate", []string{"GET"}, http.StatusNoContent},
				{"/connect/{multi-address:.+}", []string{"POST"}, http.StatusNoContent},
				{"/blocklist", []string{"GET"}, http.StatusNoContent},
				{"/peers/{address}", []string{"DELETE"}, http.StatusNoContent},
				{"/topology", []string{"GET"}, http.StatusNoContent},
				{"/welcome-message", []string{"GET", "POST"}, http.StatusNoContent},
				{"/balances", []string{"GET"}, http.StatusNoContent},
				{"/balances/{peer}", []string{"GET"}, http.StatusNoContent},
				{"/consumed", []string{"GET"}, http.StatusNoContent},
				{"/consumed/{peer}", []string{"GET"}, http.StatusNoContent},
				{"/timesettlements", []string{"GET"}, http.StatusNoContent},
				{"/settlements", []string{"GET"}, http.StatusNoContent},
				{"/settlements/{peer}", []string{"GET"}, http.StatusNoContent},
				{"/chequebook/cheque/{peer}", []string{"GET"}, http.StatusNoContent},
				{"/chequebook/cheque", []string{"GET"}, http.StatusNoContent},
				{"/chequebook/cashout/{peer}", []string{"GET", "POST"}, http.StatusNoContent},
				{"/chequebook/balance", []string{"GET"}, http.StatusNoContent},
				{"/chequebook/address", []string{"GET"}, http.StatusNoContent},
				{"/chequebook/deposit", []string{"POST"}, http.StatusNoContent},
				{"/chequebook/withdraw", []string{"POST"}, http.StatusNoContent},
				{"/wallet", []string{"GET"}, http.StatusNoContent},
				{"/wallet/withdraw/{coin}", []string{"POST"}, http.StatusNoContent},
				{"/stamps", []string{"GET"}, http.StatusNoContent},
				{"/stamps/{batch_id}", []string{"GET"}, http.StatusNoContent},
				{"/stamps/{batch_id}/buckets", []string{"GET"}, http.StatusNoContent},
				{"/stamps/{amount}/{depth}", []string{"POST"}, http.StatusNoContent},
				{"/stamps/topup/{batch_id}/{amount}", []string{"PATCH"}, http.StatusNoContent},
				{"/stamps/dilute/{batch_id}/{depth}", []string{"PATCH"}, http.StatusNoContent},
				{"/batches", []string{"GET"}, http.StatusNoContent},
				{"/accounting", []string{"GET"}, http.StatusNoContent},
				{"/stake/withdrawable", []string{"GET", "DELETE"}, http.StatusNoContent},
				{"/stake/{amount}", []string{"POST"}, http.StatusNoContent},
				{"/stake", []string{"GET", "DELETE"}, http.StatusNoContent},
				{"/redistributionstate", []string{"GET"}, http.StatusNoContent},
				{"/status", []string{"GET"}, http.StatusNoContent},
				{"/status/peers", []string{"GET"}, http.StatusNoContent},
				{"/status/neighborhoods", []string{"GET"}, http.StatusNoContent},
				{"/rchash/{depth}/{anchor1}/{anchor2}", []string{"GET"}, http.StatusNoContent},
			},
		},
		{
			name: "full_api_disabled",
			serverOptions: testServerOptions{
				FullAPIDisabled:    true,
				SwapDisabled:       false,
				ChequebookDisabled: false,
			},
			expectedStatuses: []struct {
				route           string
				expectedMethods []string
				expectedStatus  int
			}{
				// routes from mountTechnicalDebug
				{"/node", []string{"GET"}, http.StatusNoContent},
				{"/addresses", []string{"GET"}, http.StatusNoContent},
				{"/chainstate", []string{"GET"}, http.StatusNoContent},
				{"/debugstore", []string{"GET"}, http.StatusNoContent},
				{"/loggers", []string{"GET"}, http.StatusNoContent},
				{"/loggers/some-exp", []string{"GET"}, http.StatusNoContent},
				{"/loggers/some-exp/1", []string{"PUT"}, http.StatusNoContent},
				{"/readiness", nil, http.StatusBadRequest},
				{"/health", nil, http.StatusOK},
				{"/metrics", nil, http.StatusOK},
				{"/not_found", nil, http.StatusNotFound},

				// routes from mountAPI
				{"/", nil, http.StatusOK},
				{"/robots.txt", nil, http.StatusOK},
				{"/bytes", nil, http.StatusServiceUnavailable},
				{"/bytes/{address}", nil, http.StatusServiceUnavailable},
				{"/chunks", nil, http.StatusServiceUnavailable},
				{"/chunks/stream", nil, http.StatusServiceUnavailable},
				{"/chunks/{address}", nil, http.StatusServiceUnavailable},
				{"/envelope/{address}", nil, http.StatusServiceUnavailable},
				{"/soc/{owner}/{id}", nil, http.StatusServiceUnavailable},
				{"/feeds/{owner}/{topic}", nil, http.StatusServiceUnavailable},
				{"/bzz", nil, http.StatusServiceUnavailable},
				{"/grantee", nil, http.StatusServiceUnavailable},
				{"/grantee/{address}", nil, http.StatusServiceUnavailable},
				{"/bzz/{address}", nil, http.StatusServiceUnavailable},
				{"/bzz/{address}/{path:.*}", nil, http.StatusServiceUnavailable},
				{"/pss/send/{topic}/{targets}", nil, http.StatusServiceUnavailable},
				{"/pss/subscribe/{topic}", nil, http.StatusServiceUnavailable},
				{"/tags", nil, http.StatusServiceUnavailable},
				{"/tags/{id}", nil, http.StatusServiceUnavailable},
				{"/pins", nil, http.StatusServiceUnavailable},
				{"/pins/check", nil, http.StatusServiceUnavailable},
				{"/pins/{reference}", nil, http.StatusServiceUnavailable},
				{"/stewardship/{address}", nil, http.StatusServiceUnavailable},

				// routes from mountBusinessDebug
				{"/transactions", nil, http.StatusServiceUnavailable},
				{"/transactions/{hash}", nil, http.StatusServiceUnavailable},
				{"/peers", nil, http.StatusServiceUnavailable},
				{"/pingpong/{address}", nil, http.StatusServiceUnavailable},
				{"/reservestate", nil, http.StatusServiceUnavailable},
				{"/connect/{multi-address:.+}", nil, http.StatusServiceUnavailable},
				{"/blocklist", nil, http.StatusServiceUnavailable},
				{"/peers/{address}", nil, http.StatusServiceUnavailable},
				{"/topology", nil, http.StatusServiceUnavailable},
				{"/welcome-message", nil, http.StatusServiceUnavailable},
				{"/balances", nil, http.StatusServiceUnavailable},
				{"/balances/{peer}", nil, http.StatusServiceUnavailable},
				{"/consumed", nil, http.StatusServiceUnavailable},
				{"/consumed/{peer}", nil, http.StatusServiceUnavailable},
				{"/timesettlements", nil, http.StatusServiceUnavailable},
				{"/settlements", nil, http.StatusServiceUnavailable},
				{"/settlements/{peer}", nil, http.StatusServiceUnavailable},
				{"/chequebook/cheque/{peer}", nil, http.StatusServiceUnavailable},
				{"/chequebook/cheque", nil, http.StatusServiceUnavailable},
				{"/chequebook/cashout/{peer}", nil, http.StatusServiceUnavailable},
				{"/chequebook/balance", nil, http.StatusServiceUnavailable},
				{"/chequebook/address", nil, http.StatusServiceUnavailable},
				{"/chequebook/deposit", nil, http.StatusServiceUnavailable},
				{"/chequebook/withdraw", nil, http.StatusServiceUnavailable},
				{"/wallet", nil, http.StatusServiceUnavailable},
				{"/wallet/withdraw/{coin}", nil, http.StatusServiceUnavailable},
				{"/stamps", nil, http.StatusServiceUnavailable},
				{"/stamps/{batch_id}", nil, http.StatusServiceUnavailable},
				{"/stamps/{batch_id}/buckets", nil, http.StatusServiceUnavailable},
				{"/stamps/{amount}/{depth}", nil, http.StatusServiceUnavailable},
				{"/stamps/topup/{batch_id}/{amount}", nil, http.StatusServiceUnavailable},
				{"/stamps/dilute/{batch_id}/{depth}", nil, http.StatusServiceUnavailable},
				{"/batches", nil, http.StatusServiceUnavailable},
				{"/accounting", nil, http.StatusServiceUnavailable},
				{"/stake/withdrawable", nil, http.StatusServiceUnavailable},
				{"/stake/{amount}", nil, http.StatusServiceUnavailable},
				{"/stake", nil, http.StatusServiceUnavailable},
				{"/redistributionstate", nil, http.StatusServiceUnavailable},
				{"/status", nil, http.StatusServiceUnavailable},
				{"/status/peers", nil, http.StatusServiceUnavailable},
				{"/status/neighborhoods", nil, http.StatusServiceUnavailable},
				{"/rchash/{depth}/{anchor1}/{anchor2}", nil, http.StatusServiceUnavailable},
			},
		},
		{
			name: "swap_disabled",
			serverOptions: testServerOptions{
				FullAPIDisabled:    false,
				SwapDisabled:       true,
				ChequebookDisabled: false,
			},
			expectedStatuses: []struct {
				route           string
				expectedMethods []string
				expectedStatus  int
			}{
				// routes from mountTechnicalDebug
				{"/node", []string{"GET"}, http.StatusNoContent},
				{"/addresses", []string{"GET"}, http.StatusNoContent},
				{"/chainstate", []string{"GET"}, http.StatusNoContent},
				{"/debugstore", []string{"GET"}, http.StatusNoContent},
				{"/loggers", []string{"GET"}, http.StatusNoContent},
				{"/loggers/some-exp", []string{"GET"}, http.StatusNoContent},
				{"/loggers/some-exp/1", []string{"PUT"}, http.StatusNoContent},
				{"/readiness", nil, http.StatusBadRequest},
				{"/health", nil, http.StatusOK},
				{"/metrics", nil, http.StatusOK},
				{"/not_found", nil, http.StatusNotFound},

				// routes from mountAPI
				{"/", nil, http.StatusOK},
				{"/robots.txt", nil, http.StatusOK},
				{"/bytes", []string{"POST"}, http.StatusNoContent},
				{"/bytes/{address}", []string{"GET", "HEAD"}, http.StatusNoContent},
				{"/chunks", []string{"POST"}, http.StatusNoContent},
				{"/chunks/stream", nil, http.StatusBadRequest},
				{"/chunks/{address}", []string{"GET", "HEAD"}, http.StatusNoContent},
				{"/envelope/{address}", []string{"POST"}, http.StatusNoContent},
				{"/soc/{owner}/{id}", []string{"GET", "POST"}, http.StatusNoContent},
				{"/feeds/{owner}/{topic}", []string{"GET", "POST"}, http.StatusNoContent},
				{"/bzz", []string{"POST"}, http.StatusNoContent},
				{"/grantee", []string{"POST"}, http.StatusNoContent},
				{"/grantee/{address}", []string{"GET", "PATCH"}, http.StatusNoContent},
				{"/bzz/{address}", []string{"GET"}, http.StatusNoContent},
				{"/bzz/{address}/{path:.*}", []string{"GET", "HEAD"}, http.StatusNoContent},
				{"/pss/send/{topic}/{targets}", []string{"POST"}, http.StatusNoContent},
				{"/pss/subscribe/{topic}", nil, http.StatusBadRequest},
				{"/tags", []string{"GET", "POST"}, http.StatusNoContent},
				{"/tags/{id}", []string{"GET", "DELETE", "PATCH"}, http.StatusNoContent},
				{"/pins", []string{"GET"}, http.StatusNoContent},
				{"/pins/check", []string{"GET"}, http.StatusNoContent},
				{"/pins/{reference}", []string{"GET", "POST", "DELETE"}, http.StatusNoContent},
				{"/stewardship/{address}", []string{"GET", "PUT"}, http.StatusNoContent},

				// routes from mountBusinessDebug
				{"/transactions", []string{"GET"}, http.StatusNoContent},
				{"/transactions/{hash}", []string{"GET", "POST", "DELETE"}, http.StatusNoContent},
				{"/peers", []string{"GET"}, http.StatusNoContent},
				{"/pingpong/{address}", []string{"POST"}, http.StatusNoContent},
				{"/reservestate", []string{"GET"}, http.StatusNoContent},
				{"/connect/{multi-address:.+}", []string{"POST"}, http.StatusNoContent},
				{"/blocklist", []string{"GET"}, http.StatusNoContent},
				{"/peers/{address}", []string{"DELETE"}, http.StatusNoContent},
				{"/topology", []string{"GET"}, http.StatusNoContent},
				{"/welcome-message", []string{"GET", "POST"}, http.StatusNoContent},
				{"/balances", []string{"GET"}, http.StatusNoContent},
				{"/balances/{peer}", []string{"GET"}, http.StatusNoContent},
				{"/consumed", []string{"GET"}, http.StatusNoContent},
				{"/consumed/{peer}", []string{"GET"}, http.StatusNoContent},
				{"/timesettlements", []string{"GET"}, http.StatusNoContent},
				{"/settlements", nil, http.StatusNotImplemented},
				{"/settlements/{peer}", nil, http.StatusNotImplemented},
				{"/chequebook/cheque/{peer}", nil, http.StatusNotImplemented},
				{"/chequebook/cheque", nil, http.StatusNotImplemented},
				{"/chequebook/cashout/{peer}", nil, http.StatusNotImplemented},
				{"/chequebook/balance", []string{"GET"}, http.StatusNoContent},
				{"/chequebook/address", []string{"GET"}, http.StatusNoContent},
				{"/chequebook/deposit", []string{"POST"}, http.StatusNoContent},
				{"/chequebook/withdraw", []string{"POST"}, http.StatusNoContent},
				{"/wallet", nil, http.StatusNotImplemented},
				{"/wallet/withdraw/{coin}", nil, http.StatusNotImplemented},
				{"/stamps", []string{"GET"}, http.StatusNoContent},
				{"/stamps/{batch_id}", []string{"GET"}, http.StatusNoContent},
				{"/stamps/{batch_id}/buckets", []string{"GET"}, http.StatusNoContent},
				{"/stamps/{amount}/{depth}", []string{"POST"}, http.StatusNoContent},
				{"/stamps/topup/{batch_id}/{amount}", []string{"PATCH"}, http.StatusNoContent},
				{"/stamps/dilute/{batch_id}/{depth}", []string{"PATCH"}, http.StatusNoContent},
				{"/batches", []string{"GET"}, http.StatusNoContent},
				{"/accounting", []string{"GET"}, http.StatusNoContent},
				{"/stake/withdrawable", []string{"GET", "DELETE"}, http.StatusNoContent},
				{"/stake/{amount}", []string{"POST"}, http.StatusNoContent},
				{"/stake", []string{"GET", "DELETE"}, http.StatusNoContent},
				{"/redistributionstate", []string{"GET"}, http.StatusNoContent},
				{"/status", []string{"GET"}, http.StatusNoContent},
				{"/status/peers", []string{"GET"}, http.StatusNoContent},
				{"/status/neighborhoods", []string{"GET"}, http.StatusNoContent},
				{"/rchash/{depth}/{anchor1}/{anchor2}", []string{"GET"}, http.StatusNoContent},
			},
		},
		{
			name: "chechebook_disabled",
			serverOptions: testServerOptions{
				FullAPIDisabled:    false,
				SwapDisabled:       false,
				ChequebookDisabled: true,
			},
			expectedStatuses: []struct {
				route           string
				expectedMethods []string
				expectedStatus  int
			}{
				// routes from mountTechnicalDebug
				{"/node", []string{"GET"}, http.StatusNoContent},
				{"/addresses", []string{"GET"}, http.StatusNoContent},
				{"/chainstate", []string{"GET"}, http.StatusNoContent},
				{"/debugstore", []string{"GET"}, http.StatusNoContent},
				{"/loggers", []string{"GET"}, http.StatusNoContent},
				{"/loggers/some-exp", []string{"GET"}, http.StatusNoContent},
				{"/loggers/some-exp/1", []string{"PUT"}, http.StatusNoContent},
				{"/readiness", nil, http.StatusBadRequest},
				{"/health", nil, http.StatusOK},
				{"/metrics", nil, http.StatusOK},
				{"/not_found", nil, http.StatusNotFound},

				// routes from mountAPI
				{"/", nil, http.StatusOK},
				{"/robots.txt", nil, http.StatusOK},
				{"/bytes", []string{"POST"}, http.StatusNoContent},
				{"/bytes/{address}", []string{"GET", "HEAD"}, http.StatusNoContent},
				{"/chunks", []string{"POST"}, http.StatusNoContent},
				{"/chunks/stream", nil, http.StatusBadRequest},
				{"/chunks/{address}", []string{"GET", "HEAD"}, http.StatusNoContent},
				{"/envelope/{address}", []string{"POST"}, http.StatusNoContent},
				{"/soc/{owner}/{id}", []string{"GET", "POST"}, http.StatusNoContent},
				{"/feeds/{owner}/{topic}", []string{"GET", "POST"}, http.StatusNoContent},
				{"/bzz", []string{"POST"}, http.StatusNoContent},
				{"/grantee", []string{"POST"}, http.StatusNoContent},
				{"/grantee/{address}", []string{"GET", "PATCH"}, http.StatusNoContent},
				{"/bzz/{address}", []string{"GET"}, http.StatusNoContent},
				{"/bzz/{address}/{path:.*}", []string{"GET", "HEAD"}, http.StatusNoContent},
				{"/pss/send/{topic}/{targets}", []string{"POST"}, http.StatusNoContent},
				{"/pss/subscribe/{topic}", nil, http.StatusBadRequest},
				{"/tags", []string{"GET", "POST"}, http.StatusNoContent},
				{"/tags/{id}", []string{"GET", "DELETE", "PATCH"}, http.StatusNoContent},
				{"/pins", []string{"GET"}, http.StatusNoContent},
				{"/pins/check", []string{"GET"}, http.StatusNoContent},
				{"/pins/{reference}", []string{"GET", "POST", "DELETE"}, http.StatusNoContent},
				{"/stewardship/{address}", []string{"GET", "PUT"}, http.StatusNoContent},

				// routes from mountBusinessDebug
				{"/transactions", []string{"GET"}, http.StatusNoContent},
				{"/transactions/{hash}", []string{"GET", "POST", "DELETE"}, http.StatusNoContent},
				{"/peers", []string{"GET"}, http.StatusNoContent},
				{"/pingpong/{address}", []string{"POST"}, http.StatusNoContent},
				{"/reservestate", []string{"GET"}, http.StatusNoContent},
				{"/connect/{multi-address:.+}", []string{"POST"}, http.StatusNoContent},
				{"/blocklist", []string{"GET"}, http.StatusNoContent},
				{"/peers/{address}", []string{"DELETE"}, http.StatusNoContent},
				{"/topology", []string{"GET"}, http.StatusNoContent},
				{"/welcome-message", []string{"GET", "POST"}, http.StatusNoContent},
				{"/balances", []string{"GET"}, http.StatusNoContent},
				{"/balances/{peer}", []string{"GET"}, http.StatusNoContent},
				{"/consumed", []string{"GET"}, http.StatusNoContent},
				{"/consumed/{peer}", []string{"GET"}, http.StatusNoContent},
				{"/timesettlements", []string{"GET"}, http.StatusNoContent},
				{"/settlements", []string{"GET"}, http.StatusNoContent},
				{"/settlements/{peer}", []string{"GET"}, http.StatusNoContent},
				{"/chequebook/cheque/{peer}", []string{"GET"}, http.StatusNoContent},
				{"/chequebook/cheque", []string{"GET"}, http.StatusNoContent},
				{"/chequebook/cashout/{peer}", []string{"GET", "POST"}, http.StatusNoContent},
				{"/chequebook/balance", nil, http.StatusNotImplemented},
				{"/chequebook/address", nil, http.StatusNotImplemented},
				{"/chequebook/deposit", nil, http.StatusNotImplemented},
				{"/chequebook/withdraw", nil, http.StatusNotImplemented},
				{"/wallet", nil, http.StatusNotImplemented},
				{"/wallet/withdraw/{coin}", nil, http.StatusNotImplemented},
				{"/stamps", []string{"GET"}, http.StatusNoContent},
				{"/stamps/{batch_id}", []string{"GET"}, http.StatusNoContent},
				{"/stamps/{batch_id}/buckets", []string{"GET"}, http.StatusNoContent},
				{"/stamps/{amount}/{depth}", []string{"POST"}, http.StatusNoContent},
				{"/stamps/topup/{batch_id}/{amount}", []string{"PATCH"}, http.StatusNoContent},
				{"/stamps/dilute/{batch_id}/{depth}", []string{"PATCH"}, http.StatusNoContent},
				{"/batches", []string{"GET"}, http.StatusNoContent},
				{"/accounting", []string{"GET"}, http.StatusNoContent},
				{"/stake/withdrawable", []string{"GET", "DELETE"}, http.StatusNoContent},
				{"/stake/{amount}", []string{"POST"}, http.StatusNoContent},
				{"/stake", []string{"GET", "DELETE"}, http.StatusNoContent},
				{"/redistributionstate", []string{"GET"}, http.StatusNoContent},
				{"/status", []string{"GET"}, http.StatusNoContent},
				{"/status/peers", []string{"GET"}, http.StatusNoContent},
				{"/status/neighborhoods", []string{"GET"}, http.StatusNoContent},
				{"/rchash/{depth}/{anchor1}/{anchor2}", []string{"GET"}, http.StatusNoContent},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			testServer, _, _, _ := newTestServer(t, tc.serverOptions)

			routeToName := func(route string) string {
				if route == "/" {
					return "root"
				}
				return strings.ReplaceAll(route, "/", "_")
			}

			for _, tt := range tc.expectedStatuses {
				t.Run(tc.name+routeToName(tt.route), func(t *testing.T) {
					resp := jsonhttptest.Request(t, testServer, http.MethodOptions, tt.route, tt.expectedStatus)

					allowHeader := resp.Get("Allow")
					actualMethods := strings.Split(allowHeader, ", ")

					for _, expectedMethod := range tt.expectedMethods {
						if !contains(actualMethods, expectedMethod) {
							t.Errorf("expected method %s not found for route %s", expectedMethod, tt.route)
						}
					}
				})
			}
		})
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
