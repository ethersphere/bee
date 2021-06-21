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
	timeNow func() time.Time
}

const (
	overlayPrefix = "verified_overlay_"
)

func peerOverlayKey(peer swarm.Address, txHash common.Hash) string {
	return fmt.Sprintf("%s%s_%s", overlayPrefix, peer.String(), txHash.String())
}

var (
	ErrTransactionNotFound      = errors.New("transaction not found")
	ErrTransactionPending       = errors.New("transaction in pending status")
	ErrTransactionSenderInvalid = errors.New("invalid transaction sender")
	ErrGreylisted               = errors.New("overlay and transaction greylisted")
	ErrBlockHashMismatch        = errors.New("block hash mismatch")
	ErrOverlayMismatch          = errors.New("overlay mismatch")
)

type overlayVerification struct {
	NextBlockHash []byte
	Verified      bool
	TimeStamp     time.Time
}

func NewMatcher(backend Backend, signer types.Signer, storage storage.StateStorer) *Matcher {
	return &Matcher{
		storage: storage,
		backend: backend,
		signer:  signer,
		timeNow: time.Now,
	}
}

func (m *Matcher) Matches(ctx context.Context, tx []byte, networkID uint64, senderOverlay swarm.Address) ([]byte, error) {

	incomingTx := common.BytesToHash(tx)

	var val overlayVerification
	err := m.storage.Get(peerOverlayKey(senderOverlay, incomingTx), &val)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
	} else if val.Verified {
		// add cache invalidation
		return val.NextBlockHash, nil
	} else if val.TimeStamp.Add(5 * time.Minute).After(m.timeNow()) {
		return nil, fmt.Errorf("%w until %s", ErrGreylisted, val.TimeStamp.Add(5*time.Minute))
	}

	err = m.storage.Put(peerOverlayKey(senderOverlay, incomingTx), &overlayVerification{
		TimeStamp: m.timeNow(),
		Verified:  false,
	})
	if err != nil {
		return nil, err
	}

	nTx, isPending, err := m.backend.TransactionByHash(ctx, incomingTx)
	if err != nil {
		fmt.Printf("!!! %v\n", ErrTransactionNotFound)
		return nil, fmt.Errorf("%v: %w", err, ErrTransactionNotFound)
	}

	if isPending {
		return nil, ErrTransactionPending
	}

	sender, err := types.Sender(m.signer, nTx)
	if err != nil {
		fmt.Printf("!!! %v\n", ErrTransactionSenderInvalid)
		return nil, fmt.Errorf("%v: %w", err, ErrTransactionSenderInvalid)
	}

	receipt, err := m.backend.TransactionReceipt(ctx, incomingTx)
	if err != nil {
		fmt.Printf("!!! receipt %v\n", ErrTransactionSenderInvalid)
		return nil, err
	}

	nextBlock, err := m.backend.HeaderByNumber(ctx, big.NewInt(0).Add(receipt.BlockNumber, big.NewInt(1)))
	if err != nil {
		fmt.Printf("!!! nextblock\n")
		return nil, err
	}

	receiptBlockHash := receipt.BlockHash.Bytes()
	nextBlockParentHash := nextBlock.ParentHash.Bytes()
	nextBlockHash := nextBlock.Hash().Bytes()

	if !bytes.Equal(receiptBlockHash, nextBlockParentHash) {
		fmt.Printf("mismatch1\n")
		return nil, fmt.Errorf("receipt hash %x does not match block's parent hash %x: %w", receiptBlockHash, nextBlockParentHash, ErrBlockHashMismatch)
	}

	expectedRemoteBzzAddress := crypto.NewOverlayFromEthereumAddress(sender.Bytes(), networkID, nextBlockHash)

	if !expectedRemoteBzzAddress.Equal(senderOverlay) {
		fmt.Printf("mismatch2\n")
		return nil, ErrOverlayMismatch
	}

	err = m.storage.Put(peerOverlayKey(senderOverlay, incomingTx), &overlayVerification{
		TimeStamp:     m.timeNow(),
		Verified:      true,
		NextBlockHash: nextBlockHash,
	})

	if err != nil {
		return nil, err
	}

	return nextBlockHash, nil
}
