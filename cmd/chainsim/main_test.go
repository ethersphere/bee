// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethersphere/bee/v2/pkg/chainsim"
	"github.com/stretchr/testify/require"
)

func TestRPCSmoke(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	cfg.BlockPeriod = time.Hour
	sim := chainsim.New(cfg)
	defer sim.Close()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	addr := crypto.PubkeyToAddress(key.PublicKey)
	sim.SetBalance(addr, big.NewInt(1e18))

	ts := httptest.NewServer(newHTTPServer(sim, cfg.ChainID.Int64()).Handler)
	defer ts.Close()

	client, err := ethclient.Dial(ts.URL)
	require.NoError(t, err)
	defer client.Close()

	var version string
	require.NoError(t, client.Client().CallContext(context.Background(), &version, "web3_clientVersion"))
	require.Equal(t, clientVersion, version)

	chainID, err := client.ChainID(context.Background())
	require.NoError(t, err)
	require.Equal(t, cfg.ChainID, chainID)

	blockNum, err := client.BlockNumber(context.Background())
	require.NoError(t, err)
	require.Equal(t, uint64(0), blockNum)

	header, err := client.HeaderByNumber(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, header.BaseFee)

	balance, err := client.BalanceAt(context.Background(), addr, nil)
	require.NoError(t, err)
	require.Equal(t, big.NewInt(1e18), balance)
}

func TestDebugStatusEndpoint(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	sim := chainsim.New(cfg)
	defer sim.Close()

	ts := httptest.NewServer(newHTTPServer(sim, cfg.ChainID.Int64()).Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/debug/status")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var status chainsim.DebugStatus
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&status))
	require.Equal(t, uint64(0), status.BlockNumber)
	require.NotEmpty(t, status.Fees.BaseFee)
}

func TestYAMLConfigDefaults(t *testing.T) {
	t.Parallel()

	cfg := YAMLConfig{}.normalized()
	require.Equal(t, int64(1337), cfg.ChainID)
	require.Equal(t, defaultRPCListen, cfg.RPC.Endpoint)
	require.Equal(t, "chainsim-state", cfg.StateDir)
}

func TestSnapshotJSONRoundTrip(t *testing.T) {
	t.Parallel()

	sim := chainsim.New(chainsim.DefaultConfig())
	sim.SetBalance(common.HexToAddress("0x1"), big.NewInt(42))
	snap := sim.Snapshot()

	raw, err := json.Marshal(snap)
	require.NoError(t, err)

	var decoded chainsim.Snapshot
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, chainsim.SnapshotVersion, decoded.Version)
}
