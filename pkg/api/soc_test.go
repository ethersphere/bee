// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/soc"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage/mock"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
)

func TestSoc(t *testing.T) {

	var (
		socResource    = func(owner, id, sig string) string { return fmt.Sprintf("/soc/%s/%s/%s", owner, id, sig) }
		_              = testingc.GenerateTestRandomChunk()
		mockStatestore = statestore.NewStateStore()
		logger         = logging.New(ioutil.Discard, 0)
		tag            = tags.NewTags(mockStatestore, logger)
		_              = common.HexToAddress("8d3766440f0d7b949a5e32995d09619a7f86e632")
		mockStorer     = mock.NewStorer()
		client, _, _   = newTestServer(t, testServerOptions{
			Storer: mockStorer,
			Tags:   tag,
		})
	)

	t.Run("cmpty data", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, socResource("8d3766440f0d7b949a5e32995d09619a7f86e632", "bb", "cc"), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "short chunk data",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("malformed id", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, socResource("8d3766440f0d7b949a5e32995d09619a7f86e632", "bbzz", "cc"), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "bad id",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("malformed owner", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, socResource("xyz", "aa", "bb"), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "bad owner",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("malformed signature", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, socResource("8d3766440f0d7b949a5e32995d09619a7f86e632", "aa", "bad sig"), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "bad signature",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("signature invalid", func(t *testing.T) {
		s, owner, payload := mockSoc(t)
		id := make([]byte, soc.IdSize)

		// modify the sign
		sig := s.Signature()
		sig[12] = 0x98
		sig[10] = 0x12

		jsonhttptest.Request(t, client, http.MethodPost, socResource(hex.EncodeToString(owner), hex.EncodeToString(id), hex.EncodeToString(sig)), http.StatusBadRequest,
			jsonhttptest.WithRequestBody(bytes.NewReader(payload)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "invalid chunk",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("ok", func(t *testing.T) {
		s, owner, payload := mockSoc(t)
		id := make([]byte, soc.IdSize)

		sig := s.Signature()
		addr, err := s.Address()
		if err != nil {
			t.Fatal(err)
		}
		jsonhttptest.Request(t, client, http.MethodPost, socResource(hex.EncodeToString(owner), hex.EncodeToString(id), hex.EncodeToString(sig)), http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(payload)),
			jsonhttptest.WithExpectedJSONResponse(api.SocPostResponse{
				Reference: addr,
			}),
		)

		// try to fetch the same chunk
		rsrc := fmt.Sprintf("/chunks/" + addr.String())
		resp := request(t, client, http.MethodGet, rsrc, nil, http.StatusOK)
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		ch, err := s.ToChunk()
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(ch.Data(), data) {
			t.Fatal("data retrieved doesnt match uploaded content")
		}
	})
}

// returns a valid, mocked SOC
func mockSoc(t *testing.T) (*soc.Soc, []byte, []byte) {
	// create a valid soc
	id := make([]byte, soc.IdSize)
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)

	bmtHashOfFoo := "2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48"
	address := swarm.MustParseHexAddress(bmtHashOfFoo)
	foo := "foo"
	fooLength := len(foo)
	fooBytes := make([]byte, 8+fooLength)
	binary.LittleEndian.PutUint64(fooBytes, uint64(fooLength))
	copy(fooBytes[8:], foo)
	ch := swarm.NewChunk(address, fooBytes)

	sch := soc.New(id, ch)
	if err != nil {
		t.Fatal(err)
	}
	err = sch.AddSigner(signer)
	if err != nil {
		t.Fatal(err)
	}

	_, _ = sch.ToChunk()

	return sch, sch.OwnerAddress(), ch.Data()
}
