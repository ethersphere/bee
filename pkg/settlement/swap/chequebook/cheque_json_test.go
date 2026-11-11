// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"encoding/json"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/v2/pkg/statestore/storeadapter"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
)

// The golden encodings below were produced by the default encoding/json
// encoding, before Cheque and SignedCheque had their own JSON methods. Cheques
// already in the statestore and cheques sent to peers use this format, so it
// must not change.
const (
	goldenCheque       = `{"Chequebook":"0x00000000000000000000000000000000000000cb","Beneficiary":"0x00000000000000000000000000000000000000be","CumulativePayout":1267650600228229401496703205376}`
	goldenSignedCheque = `{"Chequebook":"0x00000000000000000000000000000000000000cb","Beneficiary":"0x00000000000000000000000000000000000000be","CumulativePayout":1267650600228229401496703205376,"Signature":"AQID/w=="}`
	goldenNilSignature = `{"Chequebook":"0x00000000000000000000000000000000000000cb","Beneficiary":"0x00000000000000000000000000000000000000be","CumulativePayout":1267650600228229401496703205376,"Signature":null}`
	goldenNested       = `{"TxHash":"0x0000000000000000000000000000000000000000000000000000000000000000","Cheque":` + goldenSignedCheque + `}`
)

func goldenChequeValue() chequebook.Cheque {
	return chequebook.Cheque{
		Chequebook:       common.HexToAddress("0xcb"),
		Beneficiary:      common.HexToAddress("0xbe"),
		CumulativePayout: new(big.Int).Lsh(big.NewInt(1), 100),
	}
}

func goldenSignedChequeValue() chequebook.SignedCheque {
	return chequebook.SignedCheque{
		Cheque:    goldenChequeValue(),
		Signature: []byte{1, 2, 3, 255},
	}
}

func TestChequeMarshalJSONGolden(t *testing.T) {
	t.Parallel()

	cheque := goldenChequeValue()
	signed := goldenSignedChequeValue()
	nilSignature := chequebook.SignedCheque{Cheque: goldenChequeValue()}
	// mirrors the cashout action, which holds a SignedCheque by value
	nested := struct {
		TxHash common.Hash
		Cheque chequebook.SignedCheque
	}{Cheque: signed}

	for _, tc := range []struct {
		name string
		v    any
		want string
	}{
		{name: "cheque value", v: cheque, want: goldenCheque},
		{name: "cheque pointer", v: &cheque, want: goldenCheque},
		{name: "signed cheque value", v: signed, want: goldenSignedCheque},
		{name: "signed cheque pointer", v: &signed, want: goldenSignedCheque},
		{name: "signed cheque nil signature", v: nilSignature, want: goldenNilSignature},
		{name: "signed cheque nested by value", v: nested, want: goldenNested},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := json.Marshal(tc.v)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestChequeUnmarshalJSONGolden(t *testing.T) {
	t.Parallel()

	var cheque chequebook.Cheque
	if err := json.Unmarshal([]byte(goldenCheque), &cheque); err != nil {
		t.Fatal(err)
	}
	want := goldenChequeValue()
	if !cheque.Equal(&want) {
		t.Fatalf("got %v, want %v", &cheque, &want)
	}

	var signed *chequebook.SignedCheque
	if err := json.Unmarshal([]byte(goldenSignedCheque), &signed); err != nil {
		t.Fatal(err)
	}
	wantSigned := goldenSignedChequeValue()
	if !signed.Equal(&wantSigned) {
		t.Fatalf("got %v, want %v", signed, &wantSigned)
	}

	// An issued cheque is stored as a Cheque and read back as a SignedCheque.
	var issued chequebook.SignedCheque
	if err := json.Unmarshal([]byte(goldenCheque), &issued); err != nil {
		t.Fatal(err)
	}
	if !issued.Cheque.Equal(&want) || issued.Signature != nil {
		t.Fatalf("got %v with signature %x, want %v without signature", &issued.Cheque, issued.Signature, &want)
	}

	// Field names are matched case-insensitively, as with the default decoding.
	var lower chequebook.SignedCheque
	body := `{"chequebook":"0x00000000000000000000000000000000000000cb","beneficiary":"0x00000000000000000000000000000000000000be","cumulativePayout":1267650600228229401496703205376,"signature":"AQID/w=="}`
	if err := json.Unmarshal([]byte(body), &lower); err != nil {
		t.Fatal(err)
	}
	if !lower.Equal(&wantSigned) {
		t.Fatalf("got %v, want %v", &lower, &wantSigned)
	}
}

func TestChequeUnmarshalJSONInvalidPayout(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		data string
		want error
	}{
		{name: "empty object", data: `{}`, want: chequebook.ErrNilCumulativePayout},
		{name: "null payout", data: `{"CumulativePayout":null}`, want: chequebook.ErrNilCumulativePayout},
		{name: "missing payout", data: `{"Chequebook":"0x00000000000000000000000000000000000000cb","Signature":"AQID/w=="}`, want: chequebook.ErrNilCumulativePayout},
		{name: "negative payout", data: `{"CumulativePayout":-1}`, want: chequebook.ErrNegativeCumulativePayout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var cheque chequebook.Cheque
			if err := json.Unmarshal([]byte(tc.data), &cheque); !errors.Is(err, tc.want) {
				t.Fatalf("cheque: expected %v, got %v", tc.want, err)
			}

			var signed *chequebook.SignedCheque
			err := json.Unmarshal([]byte(tc.data), &signed)
			if !errors.Is(err, tc.want) {
				t.Fatalf("signed cheque: expected %v, got %v", tc.want, err)
			}
			if errors.Is(err, chequebook.ErrNoCheque) {
				t.Fatal("invalid payout must not be reported as no cheque")
			}
		})
	}

	// Decoding into a value calls UnmarshalJSON even for a literal null.
	var signed chequebook.SignedCheque
	if err := json.Unmarshal([]byte(`null`), &signed); !errors.Is(err, chequebook.ErrNilCumulativePayout) {
		t.Fatalf("expected %v, got %v", chequebook.ErrNilCumulativePayout, err)
	}

	// A zero payout is valid.
	if err := json.Unmarshal([]byte(`{"CumulativePayout":0}`), &signed); err != nil {
		t.Fatal(err)
	}
}

func TestChequeMarshalJSONInvalidPayout(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		payout *big.Int
		want   error
	}{
		{name: "nil payout", payout: nil, want: chequebook.ErrNilCumulativePayout},
		{name: "negative payout", payout: big.NewInt(-1), want: chequebook.ErrNegativeCumulativePayout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cheque := chequebook.Cheque{CumulativePayout: tc.payout}
			if _, err := json.Marshal(cheque); !errors.Is(err, tc.want) {
				t.Fatalf("cheque: expected %v, got %v", tc.want, err)
			}

			signed := &chequebook.SignedCheque{Cheque: cheque, Signature: []byte{1}}
			if _, err := json.Marshal(signed); !errors.Is(err, tc.want) {
				t.Fatalf("signed cheque: expected %v, got %v", tc.want, err)
			}
		})
	}
}

// TestChequeStoreCorruptEntryStoreAdapter reads corrupt last-received-cheque
// entries through the statestore adapter the node uses, which decodes into an
// interface value and so follows a different encoding/json path than a direct
// json.Unmarshal.
func TestChequeStoreCorruptEntryStoreAdapter(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		data string
		want error
	}{
		{name: "null", data: `null`, want: chequebook.ErrNilCheque},
		{name: "empty object", data: `{}`, want: chequebook.ErrNilCumulativePayout},
		{name: "negative payout", data: `{"CumulativePayout":-5}`, want: chequebook.ErrNegativeCumulativePayout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store, err := storeadapter.NewStateStorerAdapter(inmemstore.New())
			if err != nil {
				t.Fatal(err)
			}
			chequebookAddr := common.HexToAddress("0xcb")
			if err := store.Put(chequebook.LastReceivedChequeKey(chequebookAddr), json.RawMessage(tc.data)); err != nil {
				t.Fatal(err)
			}

			cs := chequebook.NewChequeStore(store, nil, 1, common.HexToAddress("0xbe"), nil, nil)

			cheque, err := cs.LastCheque(chequebookAddr)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
			if errors.Is(err, chequebook.ErrNoCheque) {
				t.Fatal("corrupt entry must not be reported as no cheque")
			}
			if cheque != nil {
				t.Fatalf("expected nil cheque, got %v", cheque)
			}
		})
	}
}
