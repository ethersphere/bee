// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	transactionmock "github.com/ethersphere/bee/v2/pkg/transaction/mock"
)

// codeReaderMock satisfies chequebook.CodeReader. Returning ([]byte, nil) for
// the configured chequebook address; everything else triggers test failure.
type codeReaderMock struct {
	t        *testing.T
	expected common.Address
	code     []byte
	err      error
}

func (m *codeReaderMock) CodeAt(_ context.Context, contract common.Address, _ *big.Int) ([]byte, error) {
	m.t.Helper()
	if contract != m.expected {
		m.t.Fatalf("CodeAt: unexpected address %s, want %s", contract.Hex(), m.expected.Hex())
	}
	return m.code, m.err
}

// addressbookMock satisfies chequebook.AddressbookLookup. The (overlay, known)
// pair is returned for any chequebook address — uniqueness tests only need the
// single-mapping case.
type addressbookMock struct {
	peer  swarm.Address
	known bool
}

func (m addressbookMock) Peer(common.Address) (swarm.Address, bool) {
	return m.peer, m.known
}

// issuerCallResult ABI-packs the issuer() return value (a 20-byte address
// left-padded to 32 bytes).
func issuerCallResult(issuer common.Address) []byte {
	out := make([]byte, 32)
	copy(out[12:], issuer.Bytes())
	return out
}

// balanceCallResult ABI-packs the balance() return value.
func balanceCallResult(balance *big.Int) []byte {
	return balance.FillBytes(make([]byte, 32))
}

func TestVerifier_Success(t *testing.T) {
	t.Parallel()

	chequebookAddr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	issuer := common.HexToAddress("0x2222222222222222222222222222222222222222")
	overlay := swarm.MustParseHexAddress("aabb")
	deployedCode := []byte{0x60, 0x80, 0x60, 0x40}
	acceptedHash := ethcrypto.Keccak256Hash(deployedCode)

	tx := transactionmock.New(
		transactionmock.WithCallFunc(func(_ context.Context, req *transaction.TxRequest) ([]byte, error) {
			if req.To == nil || *req.To != chequebookAddr {
				t.Fatalf("Call: unexpected to %v", req.To)
			}
			switch req.Data[0] {
			case 0x1d: // issuer()
				return issuerCallResult(issuer), nil
			case 0xb6: // balance()
				return balanceCallResult(big.NewInt(20)), nil
			}
			t.Fatalf("Call: unexpected selector %x", req.Data[:4])
			return nil, nil
		}),
	)

	v, err := chequebook.NewVerifier(tx,
		&codeReaderMock{t: t, expected: chequebookAddr, code: deployedCode},
		addressbookMock{},
		chequebook.VerifierConfig{
			AcceptedBytecodeHashes: [][32]byte{acceptedHash},
			MinBalance:             big.NewInt(10),
		},
	)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	if err := v.Verify(context.Background(), chequebookAddr, issuer, overlay, false); err != nil {
		t.Fatalf("Verify: unexpected error: %v", err)
	}
}

func TestVerifier_IssuerMismatch(t *testing.T) {
	t.Parallel()

	chequebookAddr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	expectedIssuer := common.HexToAddress("0x2222222222222222222222222222222222222222")
	wrongIssuer := common.HexToAddress("0x3333333333333333333333333333333333333333")

	tx := transactionmock.New(
		transactionmock.WithCallFunc(func(context.Context, *transaction.TxRequest) ([]byte, error) {
			return issuerCallResult(wrongIssuer), nil
		}),
	)

	v, err := chequebook.NewVerifier(tx, &codeReaderMock{t: t, expected: chequebookAddr}, addressbookMock{}, chequebook.VerifierConfig{})
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	err = v.Verify(context.Background(), chequebookAddr, expectedIssuer, swarm.ZeroAddress, false)
	if !errors.Is(err, chequebook.ErrChequebookIssuerMismatch) {
		t.Fatalf("Verify: want ErrChequebookIssuerMismatch, got %v", err)
	}
}

func TestVerifier_BytecodeMismatch(t *testing.T) {
	t.Parallel()

	chequebookAddr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	issuer := common.HexToAddress("0x2222222222222222222222222222222222222222")
	deployedCode := []byte{0xde, 0xad, 0xbe, 0xef}
	wrongAccepted := ethcrypto.Keccak256Hash([]byte{0xca, 0xfe})

	tx := transactionmock.New(
		transactionmock.WithCallFunc(func(context.Context, *transaction.TxRequest) ([]byte, error) {
			return issuerCallResult(issuer), nil
		}),
	)

	v, err := chequebook.NewVerifier(tx,
		&codeReaderMock{t: t, expected: chequebookAddr, code: deployedCode},
		addressbookMock{},
		chequebook.VerifierConfig{AcceptedBytecodeHashes: [][32]byte{wrongAccepted}},
	)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	err = v.Verify(context.Background(), chequebookAddr, issuer, swarm.ZeroAddress, false)
	if !errors.Is(err, chequebook.ErrChequebookBytecodeMismatch) {
		t.Fatalf("Verify: want ErrChequebookBytecodeMismatch, got %v", err)
	}
}

func TestVerifier_BytecodeAcceptsMultipleVersions(t *testing.T) {
	t.Parallel()

	// Multiple accepted hashes so older deployments stay valid after a
	// master-copy upgrade.
	chequebookAddr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	issuer := common.HexToAddress("0x2222222222222222222222222222222222222222")
	v0 := []byte{0x01, 0x02}
	v1 := []byte{0x03, 0x04}

	hashes := [][32]byte{ethcrypto.Keccak256Hash(v0), ethcrypto.Keccak256Hash(v1)}

	tx := transactionmock.New(
		transactionmock.WithCallFunc(func(context.Context, *transaction.TxRequest) ([]byte, error) {
			return issuerCallResult(issuer), nil
		}),
	)

	for _, code := range [][]byte{v0, v1} {
		v, err := chequebook.NewVerifier(tx,
			&codeReaderMock{t: t, expected: chequebookAddr, code: code},
			addressbookMock{},
			chequebook.VerifierConfig{AcceptedBytecodeHashes: hashes},
		)
		if err != nil {
			t.Fatalf("NewVerifier(code=%x): %v", code, err)
		}
		if err := v.Verify(context.Background(), chequebookAddr, issuer, swarm.ZeroAddress, false); err != nil {
			t.Fatalf("Verify(code=%x): unexpected error: %v", code, err)
		}
	}
}

func TestVerifier_BytecodeCheckSkippedWhenNoHashesConfigured(t *testing.T) {
	t.Parallel()

	chequebookAddr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	issuer := common.HexToAddress("0x2222222222222222222222222222222222222222")

	tx := transactionmock.New(
		transactionmock.WithCallFunc(func(context.Context, *transaction.TxRequest) ([]byte, error) {
			return issuerCallResult(issuer), nil
		}),
	)

	// codeReader is intentionally a stub that would fail the test if invoked;
	// with no AcceptedBytecodeHashes we must not call it.
	cr := &codeReaderMock{t: t, expected: chequebookAddr, err: errors.New("must not be called")}

	v, err := chequebook.NewVerifier(tx, cr, addressbookMock{}, chequebook.VerifierConfig{})
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	if err := v.Verify(context.Background(), chequebookAddr, issuer, swarm.ZeroAddress, false); err != nil {
		t.Fatalf("Verify: unexpected error: %v", err)
	}
}

func TestVerifier_InsufficientBalance(t *testing.T) {
	t.Parallel()

	chequebookAddr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	issuer := common.HexToAddress("0x2222222222222222222222222222222222222222")

	tx := transactionmock.New(
		transactionmock.WithCallFunc(func(_ context.Context, req *transaction.TxRequest) ([]byte, error) {
			selector := req.Data[:4]
			if selector[0] == 0x1d { // issuer()
				return issuerCallResult(issuer), nil
			}
			return balanceCallResult(big.NewInt(5)), nil // balance()
		}),
	)

	v, err := chequebook.NewVerifier(tx, &codeReaderMock{t: t, expected: chequebookAddr}, addressbookMock{},
		chequebook.VerifierConfig{MinBalance: big.NewInt(10)},
	)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	err = v.Verify(context.Background(), chequebookAddr, issuer, swarm.ZeroAddress, false)
	if !errors.Is(err, chequebook.ErrChequebookInsufficientBalance) {
		t.Fatalf("Verify: want ErrChequebookInsufficientBalance, got %v", err)
	}
}

func TestVerifier_AlreadyAssociated(t *testing.T) {
	t.Parallel()

	chequebookAddr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	issuer := common.HexToAddress("0x2222222222222222222222222222222222222222")
	originalPeer := swarm.MustParseHexAddress("aabb")
	intruderPeer := swarm.MustParseHexAddress("ccdd")

	tx := transactionmock.New(
		transactionmock.WithCallFunc(func(context.Context, *transaction.TxRequest) ([]byte, error) {
			t.Fatal("Call must not be invoked: uniqueness check should fail before any RPC")
			return nil, nil
		}),
	)

	v, err := chequebook.NewVerifier(tx, &codeReaderMock{t: t, expected: chequebookAddr},
		addressbookMock{peer: originalPeer, known: true},
		chequebook.VerifierConfig{},
	)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	err = v.Verify(context.Background(), chequebookAddr, issuer, intruderPeer, false)
	if !errors.Is(err, chequebook.ErrChequebookAlreadyAssociated) {
		t.Fatalf("Verify: want ErrChequebookAlreadyAssociated, got %v", err)
	}
}

func TestVerifier_SamePeerSameChequebookAccepted(t *testing.T) {
	t.Parallel()

	// Cache hit: peer republishes with the same chequebook — issuer/bytecode
	// RPCs must be skipped entirely.
	chequebookAddr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	overlay := swarm.MustParseHexAddress("aabb")

	tx := transactionmock.New(
		transactionmock.WithCallFunc(func(context.Context, *transaction.TxRequest) ([]byte, error) {
			t.Fatal("no RPC must be issued on cache hit")
			return nil, nil
		}),
	)

	v, err := chequebook.NewVerifier(tx, &codeReaderMock{t: t, expected: chequebookAddr},
		addressbookMock{peer: overlay, known: true},
		chequebook.VerifierConfig{},
	)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	if err := v.Verify(context.Background(), chequebookAddr, common.Address{}, overlay, true); err != nil {
		t.Fatalf("Verify: unexpected error: %v", err)
	}
}

func TestVerifier_MissingAddressRejected(t *testing.T) {
	t.Parallel()

	v, err := chequebook.NewVerifier(transactionmock.New(),
		&codeReaderMock{t: t}, addressbookMock{}, chequebook.VerifierConfig{})
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	err = v.Verify(context.Background(), common.Address{}, common.Address{}, swarm.ZeroAddress, false)
	if !errors.Is(err, chequebook.ErrChequebookAddressMissing) {
		t.Fatalf("Verify: want ErrChequebookAddressMissing, got %v", err)
	}
}

// TestVerify_CacheHit_SkipsIssuerBytecodeRunsBalance verifies that when the
// addressbook already maps overlay→chequebook, Verify skips issuer() and the
// bytecode read and only re-checks balance.
func TestVerify_CacheHit_SkipsIssuerBytecodeRunsBalance(t *testing.T) {
	t.Parallel()

	chequebookAddr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	overlay := swarm.MustParseHexAddress("aabb")
	deployedCode := []byte{0x60, 0x80, 0x60, 0x40}
	acceptedHash := ethcrypto.Keccak256Hash(deployedCode)

	balanceCalls := 0
	tx := transactionmock.New(
		transactionmock.WithCallFunc(func(_ context.Context, req *transaction.TxRequest) ([]byte, error) {
			if req.To == nil || *req.To != chequebookAddr {
				t.Fatalf("Call: unexpected to %v", req.To)
			}
			switch req.Data[0] {
			case 0x1d: // issuer()
				t.Fatal("issuer() must not be called on cache hit")
			case 0xb6: // balance()
				balanceCalls++
				return balanceCallResult(big.NewInt(20)), nil
			}
			t.Fatalf("Call: unexpected selector %x", req.Data[:4])
			return nil, nil
		}),
	)

	cr := &codeReaderMock{t: t, expected: chequebookAddr, err: errors.New("must not be called")}

	v, err := chequebook.NewVerifier(tx, cr,
		addressbookMock{peer: overlay, known: true},
		chequebook.VerifierConfig{
			AcceptedBytecodeHashes: [][32]byte{acceptedHash},
			MinBalance:             big.NewInt(10),
		},
	)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	if err := v.Verify(context.Background(), chequebookAddr, common.Address{}, overlay, true); err != nil {
		t.Fatalf("Verify: unexpected error: %v", err)
	}
	if balanceCalls != 1 {
		t.Fatalf("balance() called %d times, want 1", balanceCalls)
	}
}

func TestVerify_CacheHit_BalanceBelowMin_Reject(t *testing.T) {
	t.Parallel()

	chequebookAddr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	overlay := swarm.MustParseHexAddress("aabb")

	tx := transactionmock.New(
		transactionmock.WithCallFunc(func(_ context.Context, req *transaction.TxRequest) ([]byte, error) {
			if req.Data[0] == 0x1d {
				t.Fatal("issuer() must not be called on cache hit")
			}
			return balanceCallResult(big.NewInt(5)), nil
		}),
	)

	v, err := chequebook.NewVerifier(tx, &codeReaderMock{t: t, expected: chequebookAddr},
		addressbookMock{peer: overlay, known: true},
		chequebook.VerifierConfig{MinBalance: big.NewInt(10)},
	)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	err = v.Verify(context.Background(), chequebookAddr, common.Address{}, overlay, true)
	if !errors.Is(err, chequebook.ErrChequebookInsufficientBalance) {
		t.Fatalf("Verify: want ErrChequebookInsufficientBalance, got %v", err)
	}
}

// TestVerify_CacheHit_BalanceDisabled_NoRPC asserts the cache path makes no
// RPC at all when MinBalance is nil.
func TestVerify_CacheHit_BalanceDisabled_NoRPC(t *testing.T) {
	t.Parallel()

	chequebookAddr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	overlay := swarm.MustParseHexAddress("aabb")

	tx := transactionmock.New(
		transactionmock.WithCallFunc(func(context.Context, *transaction.TxRequest) ([]byte, error) {
			t.Fatal("no RPC must be issued when MinBalance is nil")
			return nil, nil
		}),
	)

	v, err := chequebook.NewVerifier(tx, &codeReaderMock{t: t, expected: chequebookAddr},
		addressbookMock{peer: overlay, known: true},
		chequebook.VerifierConfig{},
	)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	if err := v.Verify(context.Background(), chequebookAddr, common.Address{}, overlay, true); err != nil {
		t.Fatalf("Verify: unexpected error: %v", err)
	}
}

func TestNewVerifier_NilTransactionService(t *testing.T) {
	t.Parallel()

	if _, err := chequebook.NewVerifier(nil, &codeReaderMock{t: t}, addressbookMock{}, chequebook.VerifierConfig{}); err == nil {
		t.Fatal("NewVerifier: expected error for nil transaction service, got nil")
	}
}

func TestNewVerifier_NilCodeReaderWithHashes(t *testing.T) {
	t.Parallel()

	cfg := chequebook.VerifierConfig{
		AcceptedBytecodeHashes: [][32]byte{ethcrypto.Keccak256Hash([]byte{0x01})},
	}
	if _, err := chequebook.NewVerifier(transactionmock.New(), nil, addressbookMock{}, cfg); err == nil {
		t.Fatal("NewVerifier: expected error for nil code reader with hashes set, got nil")
	}
}

func TestNewVerifier_NilCodeReaderWithoutHashesAllowed(t *testing.T) {
	t.Parallel()

	// Without accepted bytecode hashes the code reader is unused; nil is fine.
	if _, err := chequebook.NewVerifier(transactionmock.New(), nil, addressbookMock{}, chequebook.VerifierConfig{}); err != nil {
		t.Fatalf("NewVerifier: unexpected error: %v", err)
	}
}

func TestNewVerifier_NegativeMinBalance(t *testing.T) {
	t.Parallel()

	cfg := chequebook.VerifierConfig{MinBalance: big.NewInt(-1)}
	if _, err := chequebook.NewVerifier(transactionmock.New(), &codeReaderMock{t: t}, addressbookMock{}, cfg); err == nil {
		t.Fatal("NewVerifier: expected error for negative MinBalance, got nil")
	}
}
