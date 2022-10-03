// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redistributioncontract

import (
	"context"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/transaction"
)

var (
	redistributionContractABI = parseABI(redistributionABIv0_0_0)
)

const loggerName = "redistributionContract"

type Interface interface {
	ReserveSalt(context.Context) ([]byte, error)
	IsPlaying(context.Context, uint8) (bool, error)
	IsWinner(context.Context) (bool, error)
	Claim(context.Context) error
	Commit(context.Context, []byte) error
	Reveal(context.Context, uint8, []byte, []byte) error
}

type Service struct {
	overlay                   swarm.Address
	logger                    log.Logger
	txService                 transaction.Service
	incentivesContractAddress common.Address
}

func New(
	overlay swarm.Address,
	logger log.Logger,
	txService transaction.Service,
	incentivesContractAddress common.Address,
) *Service {

	s := &Service{
		overlay:                   overlay,
		logger:                    logger.WithName(loggerName).Register(),
		txService:                 txService,
		incentivesContractAddress: incentivesContractAddress,
	}
	return s
}

func (s *Service) IsPlaying(ctx context.Context, depth uint8) (bool, error) {
	callData, err := redistributionContractABI.Pack("isParticipatingInUpcomingRound", common.BytesToHash(s.overlay.Bytes()), depth)
	if err != nil {
		return false, err
	}

	result, err := s.callTx(ctx, callData)
	if err != nil {
		return false, err
	}

	results, err := redistributionContractABI.Unpack("isParticipatingInUpcomingRound", result)
	if err != nil {
		return false, err
	}

	return results[0].(bool), nil
}

func (s *Service) IsWinner(ctx context.Context) (isWinner bool, err error) {
	callData, err := redistributionContractABI.Pack("isWinner", common.BytesToHash(s.overlay.Bytes()))
	if err != nil {
		return false, err
	}

	result, err := s.callTx(ctx, callData)
	if err != nil {
		return false, err
	}

	results, err := redistributionContractABI.Unpack("isWinner", result)
	if err != nil {
		return false, err
	}

	return results[0].(bool), nil
}

func (s *Service) Claim(ctx context.Context) error {
	callData, err := redistributionContractABI.Pack("claim")
	if err != nil {
		return err
	}
	request := &transaction.TxRequest{
		To:          &s.incentivesContractAddress,
		Data:        callData,
		GasPrice:    sctx.GetGasPrice(ctx),
		GasLimit:    sctx.GetGasLimitWithDefault(ctx, 9_000_000),
		Value:       nil,
		Description: "claim win transaction",
	}
	err = s.sendAndWait(ctx, request)
	if err != nil {
		return err
	}

	return nil
}

func (s *Service) Commit(ctx context.Context, obfusHash []byte) error {
	callData, err := redistributionContractABI.Pack("commit", common.BytesToHash(obfusHash), common.BytesToHash(s.overlay.Bytes()))
	if err != nil {
		return err
	}
	request := &transaction.TxRequest{
		To:          &s.incentivesContractAddress,
		Data:        callData,
		GasPrice:    sctx.GetGasPrice(ctx),
		GasLimit:    sctx.GetGasLimitWithDefault(ctx, 3_000_000),
		Value:       nil,
		Description: "commit transaction",
	}
	err = s.sendAndWait(ctx, request)
	if err != nil {
		return err
	}

	return nil
}

func (s *Service) Reveal(ctx context.Context, storageDepth uint8, reserveCommitmentHash []byte, RandomNonce []byte) error {
	callData, err := redistributionContractABI.Pack("reveal", common.BytesToHash(s.overlay.Bytes()), storageDepth, common.BytesToHash(reserveCommitmentHash), common.BytesToHash(RandomNonce))
	if err != nil {
		return err
	}
	request := &transaction.TxRequest{
		To:          &s.incentivesContractAddress,
		Data:        callData,
		GasPrice:    sctx.GetGasPrice(ctx),
		GasLimit:    sctx.GetGasLimitWithDefault(ctx, 3_000_000),
		Value:       nil,
		Description: "reveal transaction",
	}
	err = s.sendAndWait(ctx, request)
	if err != nil {
		return err
	}

	return nil
}

func (s *Service) ReserveSalt(ctx context.Context) ([]byte, error) {
	callData, err := redistributionContractABI.Pack("currentRoundAnchor")
	if err != nil {
		return nil, err
	}

	result, err := s.callTx(ctx, callData)
	if err != nil {
		return nil, err
	}

	results, err := redistributionContractABI.Unpack("currentRoundAnchor", result)
	if err != nil {
		return nil, err
	}

	return results[0].([]byte), nil
}

func (s *Service) sendAndWait(ctx context.Context, request *transaction.TxRequest) error {
	txHash, err := s.txService.Send(ctx, request)
	if err != nil {
		return err
	}

	receipt, err := s.txService.WaitForReceipt(ctx, txHash)
	if err != nil {
		return err
	}

	if receipt.Status == 0 {
		return transaction.ErrTransactionReverted
	}
	return nil
}

func (s *Service) callTx(ctx context.Context, callData []byte) ([]byte, error) {
	result, err := s.txService.Call(ctx, &transaction.TxRequest{
		To:   &s.incentivesContractAddress,
		Data: callData,
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func parseABI(json string) abi.ABI {
	cabi, err := abi.JSON(strings.NewReader(json))
	if err != nil {
		panic(fmt.Sprintf("error creating ABI for redistribution redistributioncontract: %v", err))
	}
	return cabi
}
