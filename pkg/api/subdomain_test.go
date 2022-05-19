package api_test

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	mockpost "github.com/ethersphere/bee/pkg/postage/mock"
	resolverMock "github.com/ethersphere/bee/pkg/resolver/mock"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
)

func TestSubdomains(t *testing.T) {

	for _, tc := range []struct {
		name              string
		files             []f
		expectedReference swarm.Address
	}{
		{
			name:              "nested files with extension",
			expectedReference: swarm.MustParseHexAddress("4c9c76d63856102e54092c38a7cd227d769752d768b7adc8c3542e3dd9fcf295"),
			files: []f{
				{
					data: []byte("robots text"),
					name: "robots.txt",
					dir:  "",
					header: http.Header{
						"Content-Type": {"text/plain; charset=utf-8"},
					},
				},
				{
					data: []byte("image 1"),
					name: "1.png",
					dir:  "img",
					header: http.Header{
						"Content-Type": {"image/png"},
					},
				},
				{
					data: []byte("image 2"),
					name: "2.png",
					dir:  "img",
					header: http.Header{
						"Content-Type": {"image/png"},
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var (
				dirUploadResource   = "/bzz"
				storer              = mock.NewStorer()
				mockStatestore      = statestore.NewStateStore()
				logger              = logging.New(os.Stdout, 6)
				client, _, laddr, _ = newTestServer(t, testServerOptions{
					Storer:          storer,
					Tags:            tags.NewTags(mockStatestore, logger),
					Logger:          logger,
					PreventRedirect: true,
					Post:            mockpost.New(mockpost.WithAcceptAll()),
					Resolver: resolverMock.NewResolver(
						resolverMock.WithResolveFunc(
							func(string) (swarm.Address, error) {
								return tc.expectedReference, nil
							},
						),
					),
				})
			)

			tarReader := tarFiles(t, tc.files)

			var resp api.BzzUploadResponse

			options := []jsonhttptest.Option{
				jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
				jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
				jsonhttptest.WithRequestBody(tarReader),
				jsonhttptest.WithRequestHeader(api.SwarmCollectionHeader, "True"),
				jsonhttptest.WithRequestHeader("Content-Type", api.ContentTypeTar),
				jsonhttptest.WithUnmarshalJSONResponse(&resp),
			}

			jsonhttptest.Request(t, client, http.MethodPost, dirUploadResource, http.StatusCreated, options...)

			if resp.Reference.String() == "" {
				t.Fatalf("expected file reference, did not got any")
			}

			if tc.expectedReference.String() != resp.Reference.String() {
				t.Fatalf("got unexpected reference exp %s got %s", tc.expectedReference.String(), resp.Reference.String())
			}

			var port int
			fmt.Sscanf(laddr, "127.0.0.1:%d", &port)

			for _, f := range tc.files {
				jsonhttptest.Request(
					t, http.DefaultClient, http.MethodGet,
					fmt.Sprintf("http://test.eth.localhost:%d/%s", port, path.Join(f.dir, f.name)),
					http.StatusOK,
					jsonhttptest.WithExpectedResponse(f.data),
				)
			}
		})
	}
}
