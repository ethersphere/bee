package transaction_test

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction/backendmock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestMatchesSender(t *testing.T) {
	recipient := common.HexToAddress("0xabcd")
	txData := common.Hex2Bytes("0xabcdee")
	value := big.NewInt(1)
	suggestedGasPrice := big.NewInt(2)
	estimatedGasLimit := uint64(3)
	nonce := uint64(2)

	signedTx := types.NewTransaction(nonce, recipient, value, estimatedGasLimit, suggestedGasPrice, txData)

	t.Run("fail to retrieve tx from backend", func(t *testing.T) {
		txByHash := backendmock.WithTransactionByHashFunc(func(ctx context.Context, txHash common.Hash) (*types.Transaction, bool, error) {
			return nil, false, errors.New("transaction not found by hash")
		})

		matcher := transaction.NewMatcher(backendmock.New(txByHash), nil)

		_, err := matcher.Matches(context.Background(), []byte("0x123"), 0, swarm.NewAddress([]byte{}))
		if !errors.Is(err, transaction.ErrTransactionNotFound) {
			t.Fatalf("bad error type, want %v, got %v", transaction.ErrTransactionNotFound, err)
		}
	})

	t.Run("transaction in 'pending' status", func(t *testing.T) {
		txByHash := backendmock.WithTransactionByHashFunc(func(ctx context.Context, txHash common.Hash) (*types.Transaction, bool, error) {
			return nil, true, nil
		})

		matcher := transaction.NewMatcher(backendmock.New(txByHash), nil)

		_, err := matcher.Matches(context.Background(), []byte("0x123"), 0, swarm.NewAddress([]byte{}))
		if !errors.Is(err, transaction.ErrTransactionPending) {
			t.Fatalf("bad error type, want %v, got %v", transaction.ErrTransactionPending, err)
		}
	})

	t.Run("signer error", func(t *testing.T) {
		txByHash := backendmock.WithTransactionByHashFunc(func(ctx context.Context, txHash common.Hash) (*types.Transaction, bool, error) {
			return signedTx, false, nil
		})

		signer := &mockSigner{
			err: errors.New("can not sign"),
		}

		matcher := transaction.NewMatcher(backendmock.New(txByHash), signer)

		_, err := matcher.Matches(context.Background(), []byte("0x123"), 0, swarm.NewAddress([]byte{}))
		if !errors.Is(err, transaction.ErrTransactionSenderInvalid) {
			t.Fatalf("bad error type, want %v, got %v", transaction.ErrTransactionSenderInvalid, err)
		}
	})

	t.Run("sender does not match", func(t *testing.T) {
		txByHash := backendmock.WithTransactionByHashFunc(func(ctx context.Context, txHash common.Hash) (*types.Transaction, bool, error) {
			return signedTx, false, nil
		})

		signer := &mockSigner{
			addr: common.HexToAddress("0xabc"),
		}

		matcher := transaction.NewMatcher(backendmock.New(txByHash), signer)

		matches, err := matcher.Matches(context.Background(), []byte("0x123"), 0, swarm.NewAddress([]byte{}))
		if err != nil {
			t.Fatalf("expected no err, got %v", err)
		}

		if matches {
			t.Fatalf("expected no match, got %v", matches)
		}
	})

	t.Run("sender matches", func(t *testing.T) {
		txByHash := backendmock.WithTransactionByHashFunc(func(ctx context.Context, txHash common.Hash) (*types.Transaction, bool, error) {
			return signedTx, false, nil
		})

		signer := &mockSigner{
			addr: common.HexToAddress("0xff"),
		}

		matcher := transaction.NewMatcher(backendmock.New(txByHash), signer)

		senderOverlay := crypto.NewOverlayFromEthereumAddress(signer.addr.Bytes(), 0)

		matches, err := matcher.Matches(context.Background(), []byte("0x123"), 0, senderOverlay)
		if err != nil {
			t.Fatalf("expected no err, got %v", err)
		}

		if !matches {
			t.Fatalf("expected match, got %v", matches)
		}
	})
}

type mockSigner struct {
	addr common.Address
	err  error
}

func (m *mockSigner) Sender(tx *types.Transaction) (common.Address, error) {
	return m.addr, m.err
}

func (*mockSigner) SignatureValues(tx *types.Transaction, sig []byte) (r, s, v *big.Int, err error) {
	zero := big.NewInt(0)
	return zero, zero, zero, nil
}

func (*mockSigner) Hash(tx *types.Transaction) common.Hash {
	return common.HexToHash("0xf")
}

func (*mockSigner) Equal(types.Signer) bool {
	return false
}
