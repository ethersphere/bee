// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/bigint"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/multiformats/go-multiaddr"
)

// BeeAPI is the collection of methods exposed via the specific JSON-RPC API.
type BeeAPI struct {
	service *Service
}

// NewBeeAPI creates a new BeeAPI instance.
func NewBeeAPI(service *Service) *BeeAPI {
	return &BeeAPI{service: service}
}

// ApiVersion returns the version of the API.
func (b *BeeAPI) ApiVersion(ctx context.Context) (string, error) {
	return apiVersion, nil
}

// NodeInfo returns the node information.
func (b *BeeAPI) NodeInfo(ctx context.Context) (nodeResponse, error) {
	return b.service.nodeInfo(), nil
}

// Addresses returns the node addresses.
func (b *BeeAPI) Addresses(ctx context.Context) (*addressesResponse, error) {
	return b.service.addresses()
}

// ChainState returns the current chain state.
func (b *BeeAPI) ChainState(ctx context.Context) (*chainStateResponse, error) {
	return b.service.chainState(ctx)
}

// Peers returns the connected peers.
func (b *BeeAPI) Peers(ctx context.Context) (peersResponse, error) {
	return b.service.getPeers(), nil
}

// BlocklistedPeers returns the blocklisted peers.
func (b *BeeAPI) BlocklistedPeers(ctx context.Context) (*blockListedPeersResponse, error) {
	return b.service.blocklistedPeers()
}

// Topology returns the topology.
func (b *BeeAPI) Topology(ctx context.Context) (*topology.KadParams, error) {
	return b.service.topology()
}

// Balances returns the balances.
func (b *BeeAPI) Balances(ctx context.Context) (*balancesResponse, error) {
	return b.service.balances()
}

// Settlements returns the settlements.
func (b *BeeAPI) Settlements(ctx context.Context) (*settlementsResponse, error) {
	return b.service.settlements()
}

// ChequebookBalance returns the chequebook balance.
func (b *BeeAPI) ChequebookBalance(ctx context.Context) (*chequebookBalanceResponse, error) {
	return b.service.chequebookBalance(ctx)
}

func (b *BeeAPI) ChequebookAddress(ctx context.Context) (*chequebookAddressResponse, error) {
	return b.service.chequebookAddress(), nil
}

func (b *BeeAPI) ChequebookLastChequePeer(ctx context.Context, peer swarm.Address) (*chequebookLastChequesPeerResponse, error) {
	return b.service.chequebookLastPeer(peer)
}

func (b *BeeAPI) ChequebookLastCheques(ctx context.Context) (*chequebookLastChequesResponse, error) {
	return b.service.chequebookLastCheques()
}

func (b *BeeAPI) ChequebookCashoutStatus(ctx context.Context, peer swarm.Address) (*swapCashoutStatusResponse, error) {
	return b.service.swapCashoutStatus(ctx, peer)
}

// ChequebookDeposit deposits the amount into the chequebook
func (b *BeeAPI) ChequebookDeposit(ctx context.Context, amount *bigint.BigInt) (*chequebookTxResponse, error) {
	txHash, err := b.service.chequebookDeposit(ctx, amount.Int)
	if err != nil {
		return nil, err
	}
	return &chequebookTxResponse{TransactionHash: txHash}, nil
}

// ChequebookWithdraw withdraws the amount from the chequebook
func (b *BeeAPI) ChequebookWithdraw(ctx context.Context, amount *bigint.BigInt) (*chequebookTxResponse, error) {
	txHash, err := b.service.chequebookWithdraw(ctx, amount.Int)
	if err != nil {
		return nil, err
	}
	return &chequebookTxResponse{TransactionHash: txHash}, nil
}

// ChequebookCashout cashes out the cheque for the peer
func (b *BeeAPI) ChequebookCashout(ctx context.Context, peer swarm.Address) (*swapCashoutResponse, error) {
	txHash, err := b.service.swapCashout(ctx, peer)
	if err != nil {
		return nil, err
	}
	return &swapCashoutResponse{TransactionHash: txHash.String()}, nil
}

// WalletWithdraw withdraws the amount from the wallet
func (b *BeeAPI) WalletWithdraw(ctx context.Context, coin string, address common.Address, amount *bigint.BigInt) (*walletTxResponse, error) {
	txHash, err := b.service.walletWithdraw(ctx, coin, address, amount.Int)
	if err != nil {
		return nil, err
	}
	return &walletTxResponse{TransactionHash: txHash}, nil
}

type stakeTxResponse struct {
	TransactionHash string `json:"transactionHash"`
}

// DepositStake deposits the stake
func (b *BeeAPI) DepositStake(ctx context.Context, amount *bigint.BigInt) (*stakeTxResponse, error) {
	txHash, err := b.service.depositStake(ctx, amount.Int)
	if err != nil {
		return nil, err
	}
	return &stakeTxResponse{TransactionHash: txHash.String()}, nil
}

// WithdrawStake withdraws the stake
func (b *BeeAPI) WithdrawStake(ctx context.Context) (*stakeTxResponse, error) {
	txHash, err := b.service.withdrawStake(ctx)
	if err != nil {
		return nil, err
	}
	return &stakeTxResponse{TransactionHash: txHash.String()}, nil
}

// MigrateStake migrates the stake
func (b *BeeAPI) MigrateStake(ctx context.Context) (*stakeTxResponse, error) {
	txHash, err := b.service.migrateStake(ctx)
	if err != nil {
		return nil, err
	}
	return &stakeTxResponse{TransactionHash: txHash.String()}, nil
}

type createBatchResponse struct {
	BatchID         hexByte `json:"batchID"`
	TransactionHash string  `json:"transactionHash"`
}

// CreateBatch creates a new batch
func (b *BeeAPI) CreateBatch(ctx context.Context, amount *bigint.BigInt, depth uint8, immutable bool, label string) (*createBatchResponse, error) {
	txHash, batchID, err := b.service.createBatch(ctx, amount.Int, depth, immutable, label)
	if err != nil {
		return nil, err
	}
	return &createBatchResponse{BatchID: batchID, TransactionHash: txHash.String()}, nil
}

// TopupBatch top-ups a batch
func (b *BeeAPI) TopupBatch(ctx context.Context, batchID hexByte, amount *bigint.BigInt) (*createBatchResponse, error) {
	txHash, err := b.service.topupBatch(ctx, batchID, amount.Int)
	if err != nil {
		return nil, err
	}
	return &createBatchResponse{BatchID: batchID, TransactionHash: txHash.String()}, nil
}

// DiluteBatch dilutes a batch
func (b *BeeAPI) DiluteBatch(ctx context.Context, batchID hexByte, depth uint8) (*createBatchResponse, error) {
	txHash, err := b.service.diluteBatch(ctx, batchID, depth)
	if err != nil {
		return nil, err
	}
	return &createBatchResponse{BatchID: batchID, TransactionHash: txHash.String()}, nil
}

// Transactions returns the list of pending transactions
func (b *BeeAPI) Transactions(ctx context.Context) (*transactionPendingList, error) {
	txs, err := b.service.pendingTransactions()
	if err != nil {
		return nil, err
	}
	return &transactionPendingList{PendingTransactions: txs}, nil
}

// TransactionDetail returns the transaction info for a given hash
func (b *BeeAPI) TransactionDetail(ctx context.Context, hash common.Hash) (*transactionInfo, error) {
	return b.service.transactionDetail(hash)
}

// ResendTransaction resends a transaction
func (b *BeeAPI) ResendTransaction(ctx context.Context, hash common.Hash) (*transactionHashResponse, error) {
	err := b.service.resendTransaction(ctx, hash)
	if err != nil {
		return nil, err
	}
	return &transactionHashResponse{TransactionHash: hash}, nil
}

// CancelTransaction cancels a transaction
func (b *BeeAPI) CancelTransaction(ctx context.Context, hash common.Hash) (*transactionHashResponse, error) {
	txHash, err := b.service.cancelTransaction(ctx, hash)
	if err != nil {
		return nil, err
	}
	return &transactionHashResponse{TransactionHash: txHash}, nil
}

// TimeToLive returns the time to live for a tag.
func (b *BeeAPI) Tag(ctx context.Context, tagID uint64) (*tagResponse, error) {
	return b.service.getTag(tagID)
}

// ListTags returns the list of tags.
func (b *BeeAPI) ListTags(ctx context.Context, offset, limit int) (*listTagsResponse, error) {
	return b.service.listTags(offset, limit)
}

// Pin returns the pinned root hash.
func (b *BeeAPI) Pin(ctx context.Context, ref swarm.Address) (*struct {
	Reference swarm.Address `json:"reference"`
}, error) {
	return b.service.getPin(ref)
}

// ListPins returns the list of pinned root hashes.
func (b *BeeAPI) ListPins(ctx context.Context) (*struct {
	References []swarm.Address `json:"references"`
}, error) {
	return b.service.listPins()
}

// IsRetrievable checks if the content at the given address is retrievable.
func (b *BeeAPI) IsRetrievable(ctx context.Context, ref swarm.Address) (bool, error) {
	return b.service.isRetrievable(ctx, ref)
}

// CreateTag creates a new tag.
func (b *BeeAPI) CreateTag(ctx context.Context) (*tagResponse, error) {
	return b.service.createTag()
}

// DeleteTag deletes a tag.
func (b *BeeAPI) DeleteTag(ctx context.Context, tagID uint64) error {
	return b.service.deleteTag(tagID)
}

// DoneSplit splits a tag.
func (b *BeeAPI) DoneSplit(ctx context.Context, tagID uint64, address swarm.Address) error {
	return b.service.doneSplit(ctx, tagID, address)
}

// PinRootHash pins a root hash.
func (b *BeeAPI) PinRootHash(ctx context.Context, ref swarm.Address) error {
	return b.service.pin(ctx, ref)
}

// UnpinRootHash unpins a root hash.
func (b *BeeAPI) UnpinRootHash(ctx context.Context, ref swarm.Address) error {
	return b.service.unpin(ctx, ref)
}

// Connect connects to a peer.
func (b *BeeAPI) Connect(ctx context.Context, ma string) (*peerConnectResponse, error) {
	addr, err := multiaddr.NewMultiaddr(ma)
	if err != nil {
		return nil, err
	}
	overlay, err := b.service.connect(ctx, addr)
	if err != nil {
		return nil, err
	}
	return &peerConnectResponse{Address: overlay.String()}, nil
}

// Disconnect disconnects from a peer.
func (b *BeeAPI) Disconnect(ctx context.Context, overlay swarm.Address) error {
	return b.service.disconnect(overlay)
}
