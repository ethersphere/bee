package transaction

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type Matcher struct {
	backend Backend
	storage storage.StateStorer
	signer  types.Signer
}

const (
	overlayPrefix = "verified_overlay_"
)

func peerOverlayKey(peer swarm.Address) string {
	return fmt.Sprintf("%s%s", overlayPrefix, peer.String())
}

var (
	ErrTransactionNotFound      = errors.New("transaction not found")
	ErrTransactionPending       = errors.New("transaction in pending status")
	ErrTransactionSenderInvalid = errors.New("invalid transaction sender")
	ErrBlockHashMismatch        = errors.New("block hash mismatch")
	ErrOverlayMismatch          = errors.New("overlay mismatch")
)

type overlayVerification struct {
	nextBlockHash []byte
	verified      bool
	timeStamp     time.Time
}

func NewMatcher(backend Backend, signer types.Signer, storage storage.StateStorer) *Matcher {
	return &Matcher{
		storage: storage,
		backend: backend,
		signer:  signer,
	}
}

func (m *Matcher) Matches(ctx context.Context, tx []byte, networkID uint64, senderOverlay swarm.Address) ([]byte, error) {

	var val overlayVerification

	err := m.storage.Get(peerOverlayKey(senderOverlay), &val)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	} else {
		// add cache invalidation
		if val.verified {
			return val.nextBlockHash, nil
		}
	}

	incomingTx := common.BytesToHash(tx)

	nTx, isPending, err := m.backend.TransactionByHash(ctx, incomingTx)
	if err != nil {
		return nil, fmt.Errorf("%v: %w", err, ErrTransactionNotFound)
	}

	if isPending {
		return nil, ErrTransactionPending
	}

	sender, err := types.Sender(m.signer, nTx)
	if err != nil {
		return nil, fmt.Errorf("%v: %w", err, ErrTransactionSenderInvalid)
	}

	receipt, err := m.backend.TransactionReceipt(ctx, incomingTx)
	if err != nil {
		return nil, err
	}

	nextBlock, err := m.backend.HeaderByNumber(ctx, big.NewInt(0).Add(receipt.BlockNumber, big.NewInt(1)))
	if err != nil {
		return nil, err
	}

	receiptBlockHash := receipt.BlockHash.Bytes()
	nextBlockParentHash := nextBlock.ParentHash.Bytes()
	nextBlockHash := nextBlock.Hash().Bytes()

	if !bytes.Equal(receiptBlockHash, nextBlockParentHash) {
		return nil, fmt.Errorf("receipt hash %x does not match block's parent hash %x: %w", receiptBlockHash, nextBlockParentHash, ErrBlockHashMismatch)
	}

	expectedRemoteBzzAddress := crypto.NewOverlayFromEthereumAddress(sender.Bytes(), networkID, nextBlockHash)

	if !expectedRemoteBzzAddress.Equal(senderOverlay) {
		return nil, ErrOverlayMismatch
	}

	err = m.storage.Put(peerOverlayKey(senderOverlay), &overlayVerification{
		timeStamp:     time.Now(),
		verified:      true,
		nextBlockHash: nextBlockHash,
	})

	return nextBlockHash, nil
}
