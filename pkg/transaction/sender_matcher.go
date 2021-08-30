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
	backend    Backend
	rpcBackend RPCBackend
	storage    storage.StateStorer
	signer     types.Signer
	timeNow    func() time.Time
}

const (
	overlayPrefix = "verified_overlay_"
)

func peerOverlayKey(peer swarm.Address, txHash common.Hash) string {
	return fmt.Sprintf("%s%s_%s", overlayPrefix, peer.ByteString(), txHash.String())
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

func NewMatcher(backend Backend, rpcBackend RPCBackend, signer types.Signer, storage storage.StateStorer) *Matcher {
	return &Matcher{
		storage:    storage,
		backend:    backend,
		rpcBackend: rpcBackend,
		signer:     signer,
		timeNow:    time.Now,
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
		return nil, ErrGreylisted
	}

	nTx, isPending, err := m.backend.TransactionByHash(ctx, incomingTx)
	if err != nil {
		err2 := m.storage.Put(peerOverlayKey(senderOverlay, incomingTx), &overlayVerification{
			TimeStamp: m.timeNow(),
			Verified:  false,
		})
		if err2 != nil {
			return nil, err2
		}
		return nil, fmt.Errorf("%v: %w", err, ErrTransactionNotFound)
	}

	if isPending {
		err2 := m.storage.Put(peerOverlayKey(senderOverlay, incomingTx), &overlayVerification{
			TimeStamp: m.timeNow(),
			Verified:  false,
		})
		if err2 != nil {
			return nil, err2
		}
		return nil, ErrTransactionPending
	}

	// if transaction data is exactly one word and starts with 4 0-bytes this is a transaction data type proof
	// we check for the starting 0-bytes so we don't mistake a 28 byte data solidity call for this
	// otherwise this is considered as a signer based proof. a transaction can be only one of the two.
	var attestedOverlay common.Address
	txData := nTx.Data()
	if len(txData) == 32 && bytes.Equal(txData[0:4], []byte{0, 0, 0, 0}) {
		attestedOverlay = common.BytesToAddress(nTx.Data())
	} else {
		attestedOverlay, err = types.Sender(m.signer, nTx)
		if err != nil {
			err2 := m.storage.Put(peerOverlayKey(senderOverlay, incomingTx), &overlayVerification{
				TimeStamp: m.timeNow(),
				Verified:  false,
			})
			if err2 != nil {
				return nil, err2
			}
			return nil, fmt.Errorf("%v: %w", err, ErrTransactionSenderInvalid)
		}
	}

	receipt, err := m.backend.TransactionReceipt(ctx, incomingTx)
	if err != nil {
		err2 := m.storage.Put(peerOverlayKey(senderOverlay, incomingTx), &overlayVerification{
			TimeStamp: m.timeNow(),
			Verified:  false,
		})
		if err2 != nil {
			return nil, err2
		}
		return nil, err
	}

	nextBlockNumber := big.NewInt(0).Add(receipt.BlockNumber, big.NewInt(1))
	nextBlock, err := m.backend.HeaderByNumber(ctx, nextBlockNumber)
	if err != nil {
		err2 := m.storage.Put(peerOverlayKey(senderOverlay, incomingTx), &overlayVerification{
			TimeStamp: m.timeNow(),
			Verified:  false,
		})
		if err2 != nil {
			return nil, err2
		}
		return nil, err
	}

	receiptBlockHash := receipt.BlockHash.Bytes()
	nextBlockParentHash := nextBlock.ParentHash.Bytes()
	nextBlockHashFromBackend, err := m.rpcBackend.BlockHashAt(ctx, nextBlockNumber)
	if err != nil {
		return nil, err
	}
	nextBlockHash := nextBlockHashFromBackend.Bytes()

	if !bytes.Equal(receiptBlockHash, nextBlockParentHash) {
		err2 := m.storage.Put(peerOverlayKey(senderOverlay, incomingTx), &overlayVerification{
			TimeStamp: m.timeNow(),
			Verified:  false,
		})
		if err2 != nil {
			return nil, err2
		}
		return nil, fmt.Errorf("receipt hash %x does not match block's parent hash %x: %w", receiptBlockHash, nextBlockParentHash, ErrBlockHashMismatch)
	}

	expectedRemoteBzzAddress := crypto.NewOverlayFromEthereumAddress(attestedOverlay.Bytes(), networkID, nextBlockHash)

	if !expectedRemoteBzzAddress.Equal(senderOverlay) {
		err2 := m.storage.Put(peerOverlayKey(senderOverlay, incomingTx), &overlayVerification{
			TimeStamp: m.timeNow(),
			Verified:  false,
		})
		if err2 != nil {
			return nil, err2
		}
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
