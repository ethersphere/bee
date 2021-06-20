package transaction_test

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/crypto"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/bee/pkg/transaction/backendmock"
)

func TestMatchesSender(t *testing.T) {
	recipient := common.HexToAddress("0xabcd")
	txData := common.Hex2Bytes("0xabcdee")
	value := big.NewInt(1)
	suggestedGasPrice := big.NewInt(2)
	estimatedGasLimit := uint64(3)
	nonce := uint64(2)
	trx := common.HexToAddress("0x1").Bytes()

	signedTx := types.NewTransaction(nonce, recipient, value, estimatedGasLimit, suggestedGasPrice, txData)

	t.Run("fail to retrieve tx from backend", func(t *testing.T) {
		txByHash := backendmock.WithTransactionByHashFunc(func(ctx context.Context, txHash common.Hash) (*types.Transaction, bool, error) {
			return nil, false, errors.New("transaction not found by hash")
		})

		matcher := transaction.NewMatcher(backendmock.New(txByHash), nil, statestore.NewStateStore())

		_, err := matcher.Matches(context.Background(), trx, 0, swarm.NewAddress([]byte{}))
		if !errors.Is(err, transaction.ErrTransactionNotFound) {
			t.Fatalf("bad error type, want %v, got %v", transaction.ErrTransactionNotFound, err)
		}
	})

	t.Run("transaction in 'pending' status", func(t *testing.T) {
		txByHash := backendmock.WithTransactionByHashFunc(func(ctx context.Context, txHash common.Hash) (*types.Transaction, bool, error) {
			return nil, true, nil
		})

		matcher := transaction.NewMatcher(backendmock.New(txByHash), nil, statestore.NewStateStore())

		_, err := matcher.Matches(context.Background(), trx, 0, swarm.NewAddress([]byte{}))
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
		matcher := transaction.NewMatcher(backendmock.New(txByHash), signer, statestore.NewStateStore())

		_, err := matcher.Matches(context.Background(), trx, 0, swarm.NewAddress([]byte{}))
		if !errors.Is(err, transaction.ErrTransactionSenderInvalid) {
			t.Fatalf("bad error type, want %v, got %v", transaction.ErrTransactionSenderInvalid, err)
		}
	})

	t.Run("sender does not match", func(t *testing.T) {

		block := common.HexToHash("0x1")
		wrongParent := common.HexToHash("0x2")

		txByHash := backendmock.WithTransactionByHashFunc(func(ctx context.Context, txHash common.Hash) (*types.Transaction, bool, error) {
			return signedTx, false, nil
		})

		signer := &mockSigner{
			addr: common.HexToAddress("0xabc"),
		}

		trxReceipt := backendmock.WithTransactionReceiptFunc(func(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
			return &types.Receipt{
				BlockNumber: big.NewInt(0),
				BlockHash:   block,
			}, nil
		})

		headerByNum := backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
			return &types.Header{
				ParentHash: wrongParent,
			}, nil
		})

		matcher := transaction.NewMatcher(backendmock.New(txByHash, trxReceipt, headerByNum), signer, statestore.NewStateStore())

		_, err := matcher.Matches(context.Background(), trx, 0, swarm.NewAddress([]byte{}))
		if err == nil {
			t.Fatalf("expected no match")
		}
	})

	t.Run("sender matches", func(t *testing.T) {

		trxBlock := common.HexToHash("0x2")
		nextBlockHeader := &types.Header{
			ParentHash: trxBlock,
		}

		trxReceipt := backendmock.WithTransactionReceiptFunc(func(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
			return &types.Receipt{
				BlockNumber: big.NewInt(0),
				BlockHash:   trxBlock,
			}, nil
		})

		headerByNum := backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
			return nextBlockHeader, nil
		})

		txByHash := backendmock.WithTransactionByHashFunc(func(ctx context.Context, txHash common.Hash) (*types.Transaction, bool, error) {
			return signedTx, false, nil
		})

		signer := &mockSigner{
			addr: common.HexToAddress("0xff"),
		}

		matcher := transaction.NewMatcher(backendmock.New(trxReceipt, headerByNum, txByHash), signer, statestore.NewStateStore())

		senderOverlay := crypto.NewOverlayFromEthereumAddress(signer.addr.Bytes(), 0, nextBlockHeader.Hash().Bytes())

		_, err := matcher.Matches(context.Background(), trx, 0, senderOverlay)
		if err != nil {
			t.Fatalf("expected match")
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
