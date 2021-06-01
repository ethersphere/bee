// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swapprotocol_test

import (
        "bytes"
        "context"
        "math/big"

        "github.com/ethereum/go-ethereum/common"
        "github.com/ethersphere/bee/pkg/logging"
        "github.com/ethersphere/bee/pkg/p2p"
        "github.com/ethersphere/bee/pkg/p2p/protobuf"
        "github.com/ethersphere/bee/pkg/p2p/streamtest"
        "github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
        exchangemock "github.com/ethersphere/bee/pkg/settlement/swap/exchange/mock"
        swapmock "github.com/ethersphere/bee/pkg/settlement/swap/mock"
        "github.com/ethersphere/bee/pkg/settlement/swap/swapprotocol"
        "github.com/ethersphere/bee/pkg/settlement/swap/swapprotocol/pb"
        "github.com/ethersphere/bee/pkg/swarm"
        "io/ioutil"
        "testing"
)

/*

func TestInit(t *testing.T) {
        logger := logging.New(ioutil.Discard, 0)
        commonAddr := common.HexToAddress("0xab")
        swapHsReceiver := swapmock.NewProtocolService()
        swapHsInitiator := swapmock.NewProtocolService()
        swappHsReceiver := swapprotocol.New(nil, logger, commonAddr)
        swappHsReceiver.SetSwap(swapHsReceiver)
        recorder := streamtest.New(
                streamtest.WithProtocols(swappHsReceiver.Protocol()),
        )
        commonAddr2 := common.HexToAddress("0xdc")
        swappHsInitiator := swapprotocol.New(recorder, logger, commonAddr2)
        swappHsInitiator.SetSwap(swapHsInitiator)
        peerID := swarm.MustParseHexAddress("9ee7add7")
        peer := p2p.Peer{Address: peerID}
        if err := swappHsInitiator.Init(context.Background(), peer); err != nil {
                t.Fatal("bad")
        }
        records, err := recorder.Records(peerID, "swap", "1.0.0", "init")
        if err != nil {
                t.Fatal(err)
        }
        if l := len(records); l != 1 {
                t.Fatalf("got %v records, want %v", l, 1)
        }
        record := records[0]
        messages, err := protobuf.ReadMessages(
                bytes.NewReader(record.In()),
                func() protobuf.Message { return new(pb.Handshake) },
        )
        if err != nil {
                t.Fatal(err)
        }
        gotBeneficiary := messages[0].(*pb.Handshake).Beneficiary
        if bytes.Equal(gotBeneficiary, commonAddr2.Bytes()) {
                t.Fatalf("got %v bytes, want %v bytes", gotBeneficiary, commonAddr2.Bytes())
        }
        if len(messages) != 1 {
                t.Fatalf("got %v messages, want %v", len(messages), 1)
        }
        messages, err = protobuf.ReadMessages(
                bytes.NewReader(record.Out()),
                func() protobuf.Message { return new(pb.Handshake) },
        )
        if err != nil {
                t.Fatal(err)
        }
        gotBeneficiary = messages[0].(*pb.Handshake).Beneficiary
        if bytes.Equal(gotBeneficiary, commonAddr.Bytes()) {
                t.Fatalf("got %v bytes, want %v bytes", gotBeneficiary, commonAddr.Bytes())
        }
        if len(messages) != 1 {
                t.Fatalf("got %v messages, want %v", len(messages), 1)
        }
}

*/

func TestEmitCheque(t *testing.T) {
        logger := logging.New(ioutil.Discard, 0)
        commonAddr := common.HexToAddress("0xab")
        swapHsReceiver := swapmock.NewSwap()
        swapHsInitiator := swapmock.NewSwap()
        exchange := exchangemock.New(big.NewInt(50), big.NewInt(500))
        swappHsReceiver := swapprotocol.New(nil, logger, commonAddr, exchange)
        swappHsReceiver.SetSwap(swapHsReceiver)
        recorder := streamtest.New(
                streamtest.WithProtocols(swappHsReceiver.Protocol()),
        )
        commonAddr2 := common.HexToAddress("0xdc")
        swappHsInitiator := swapprotocol.New(recorder, logger, commonAddr2, exchange)
        swappHsInitiator.SetSwap(swapHsInitiator)
        peerID := swarm.MustParseHexAddress("9ee7add7")
        peer := p2p.Peer{Address: peerID}

        chequeAmount := big.NewInt(1250)

        issueFunc := func(ctx context.Context, beneficiary common.Address, amount *big.Int, sendChequeFunc chequebook.SendChequeFunc) (*big.Int, error) {
                cheque := &chequebook.SignedCheque{
                        Cheque: chequebook.Cheque{
                                Beneficiary:      commonAddr,
                                CumulativePayout: big.NewInt(1250),
                                Chequebook:       common.Address{},
                        },
                        Signature: []byte{},
                }
                _ = sendChequeFunc(cheque)
                return big.NewInt(3750), nil
        }

        if _, err := swappHsInitiator.EmitCheque(context.Background(), peer.Address, commonAddr, chequeAmount, issueFunc); err != nil {
                t.Fatalf("expected no error, got %v", err)
        }
        records, err := recorder.Records(peerID, "swap", "1.0.0", "swap")
        if err != nil {
                t.Fatal(err)
        }
        if l := len(records); l != 1 {
                t.Fatalf("got %v records, want %v", l, 1)
        }
        record := records[0]
        messages, err := protobuf.ReadMessages(
                bytes.NewReader(record.In()),
                func() protobuf.Message { return new(pb.EmitCheque) },
        )
        // gotCheque := messages[0].(*pb.EmitCheque)
        // Todo comparing field values in this response
        if err != nil {
                t.Fatal(err)
        }
        if len(messages) != 1 {
                t.Fatalf("got %v messages, want %v", len(messages), 1)
        }

}
