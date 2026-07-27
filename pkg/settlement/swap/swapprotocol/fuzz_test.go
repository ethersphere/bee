// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swapprotocol_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	swapmock "github.com/ethersphere/bee/v2/pkg/settlement/swap/mock"
	priceoraclemock "github.com/ethersphere/bee/v2/pkg/settlement/swap/priceoracle/mock"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/swapprotocol"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/swapprotocol/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzEmitChequeRead drives arbitrary bytes through the exact read path the swap
// handler uses for an incoming cheque: the length-delimited protobuf reader
// followed by JSON-decoding the embedded cheque.
func FuzzEmitChequeRead(f *testing.F) {
	seedCheque, err := json.Marshal(&chequebook.SignedCheque{Signature: make([]byte, 65)})
	if err != nil {
		f.Fatal(err)
	}
	var buf bytes.Buffer
	if err := protobuf.NewWriter(&buf).WriteMsg(&pb.EmitCheque{Cheque: seedCheque}); err != nil {
		f.Fatal(err)
	}
	f.Add(buf.Bytes())
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var req pb.EmitCheque
		if err := r.ReadMsg(&req); err != nil {
			return
		}
		var signedCheque *chequebook.SignedCheque
		_ = json.Unmarshal(req.Cheque, &signedCheque)
	})
}

// FuzzHandlerEmitCheque drives fuzzed wire messages end-to-end through the real
// swapprotocol stream handler and into the ReceiveCheque sink (NIL-06).
// It asserts the handler never crashes or panics on arbitrary wire payloads,
// particularly JSON literal "null", empty bodies, or missing fields.
func FuzzHandlerEmitCheque(f *testing.F) {
	validSignedCheque, err := json.Marshal(&chequebook.SignedCheque{
		Cheque: chequebook.Cheque{
			Beneficiary:      common.HexToAddress("0xab"),
			CumulativePayout: big.NewInt(1000),
			Chequebook:       common.HexToAddress("0xcd"),
		},
		Signature: make([]byte, 65),
	})
	if err != nil {
		f.Fatal(err)
	}

	// Seed 1: valid cheque JSON in protobuf
	var buf1 bytes.Buffer
	if err := protobuf.NewWriter(&buf1).WriteMsg(&pb.EmitCheque{Cheque: validSignedCheque}); err != nil {
		f.Fatal(err)
	}
	f.Add(buf1.Bytes())

	// Seed 2: JSON literal null in protobuf (finding NIL-06 crasher)
	var buf2 bytes.Buffer
	if err := protobuf.NewWriter(&buf2).WriteMsg(&pb.EmitCheque{Cheque: []byte("null")}); err != nil {
		f.Fatal(err)
	}
	f.Add(buf2.Bytes())

	// Seed 3: JSON empty string in protobuf
	var buf3 bytes.Buffer
	if err := protobuf.NewWriter(&buf3).WriteMsg(&pb.EmitCheque{Cheque: []byte(`""`)}); err != nil {
		f.Fatal(err)
	}
	f.Add(buf3.Bytes())

	// Seed 4: empty bytes
	f.Add([]byte{})

	// Seed 5: empty protobuf message
	var buf5 bytes.Buffer
	if err := protobuf.NewWriter(&buf5).WriteMsg(&pb.EmitCheque{}); err != nil {
		f.Fatal(err)
	}
	f.Add(buf5.Bytes())

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64*1024 {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		peerID := swarm.MustParseHexAddress("9ee7add7")
		commonAddr := common.HexToAddress("0xab")
		priceOracle := priceoraclemock.New(big.NewInt(50), big.NewInt(500))

		var sinkReceived atomic.Bool
		swapService := swapmock.NewSwap(
			swapmock.WithReceiveChequeFunc(func(ctx context.Context, peer swarm.Address, cheque *chequebook.SignedCheque, exchangeRate, deduction *big.Int) error {
				sinkReceived.Store(true)
				if cheque == nil {
					t.Fatal("NIL-06: ReceiveCheque sink called with nil signedCheque!")
				}
				_ = cheque.Beneficiary
				return nil
			}),
		)

		swapp := swapprotocol.New(nil, log.Noop, commonAddr, priceOracle)
		swapp.SetSwap(swapService)

		server := streamtest.New(
			streamtest.WithProtocols(swapp.Protocol()),
			streamtest.WithBaseAddr(peerID),
		)

		stream, err := server.NewStream(ctx, peerID, p2p.Headers{
			"exchange":  []byte{0x01},
			"deduction": []byte{0x00},
		}, "swap", "1.0.0", "swap")
		if err != nil {
			return
		}
		defer func() { _ = stream.Reset() }()

		_, writeErr := stream.Write(data)
		if writeErr != nil {
			return
		}
		_ = stream.Close()
		// Allow handler to complete processing
		time.Sleep(10 * time.Millisecond)
	})
}
