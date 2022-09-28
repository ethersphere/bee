// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package incentives

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/go-sw3-abi/sw3abi"
)

var (
	//TODO: add correct ABI of redistribution contract
	incentivesContractABI = parseABI(sw3abi.ERC20ABIv0_3_1)

	ErrHasNoStake = errors.New("has no stake")
)

// current round anchor returns current neighbourhood and reserve commitment
func (s *Service) IsPlaying(ctx context.Context, depth uint8) (bool, error) {
	callData, err := incentivesContractABI.Pack("isParticipatingInUpcomingRound", s.overlay, depth)
	if err != nil {
		return false, err
	}

	result, err := s.txService.Call(ctx, &transaction.TxRequest{
		To:   &s.incentivesContractAddress,
		Data: callData,
	})
	if err != nil {
		return false, err
	}

	results, err := incentivesContractABI.Unpack("isParticipatingInUpcomingRound", result)
	if err != nil {
		return false, err
	}

	return results[0].(bool), nil
}

func (s *Service) IsWinner(ctx context.Context) (isSlashed bool, isWinner bool, err error) {
	//TODO: Check if is Slashed,

	// fetches the stake for given overlay, if stake is 0, return an error
	stake, err := s.stakingContract.GetStake(ctx, s.overlay)
	if len(stake.Bits()) == 0 {
		return false, false, ErrHasNoStake
	}

	//winnerSeed, truthSeed, err := s.checkIsWinning(ctx)
	//if err != nil {
	//	return false, false, err
	//}

	callData, err := incentivesContractABI.Pack("isWinner", s.overlay)
	if err != nil {
		return false, false, err
	}

	result, err := s.txService.Call(ctx, &transaction.TxRequest{
		To:   &s.incentivesContractAddress,
		Data: callData,
	})
	if err != nil {
		return false, false, err
	}

	results, err := incentivesContractABI.Unpack("isWinner", result)
	if err != nil {
		return false, false, err
	}

	return false, results[0].(bool), nil
}

func (s *Service) Claim(ctx context.Context) error {
	callData, err := incentivesContractABI.Pack("claim")

	request := &transaction.TxRequest{
		To:   &s.incentivesContractAddress,
		Data: callData,
	}

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

	//TODO: do something with receipt

	is, err := s.neighbourSelected(ctx)
	if err != nil {
		return err
	}
	if is {
		//TODO: start sampler
		//TODO: it also starts the revealer process if neighborhood is selected
	}
	return nil
}

func (s *Service) Commit(ctx context.Context, obfusHash []byte) error {
	callData, err := incentivesContractABI.Pack("commit", s.overlay, obfusHash)

	request := &transaction.TxRequest{
		To:   &s.incentivesContractAddress,
		Data: callData,
	}

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

func (s *Service) Reveal(ctx context.Context, storageDepth uint8, reserveCommitmentHash []byte, RandomNonce []byte) error {
	callData, err := incentivesContractABI.Pack("reveal", s.overlay, storageDepth, reserveCommitmentHash, RandomNonce)

	request := &transaction.TxRequest{
		To:   &s.incentivesContractAddress,
		Data: callData,
	}

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

func (s Service) ReserveSalt(ctx context.Context) ([]byte, error) {
	callData, err := incentivesContractABI.Pack("currentRoundAnchor")
	if err != nil {
		return nil, err
	}

	result, err := s.txService.Call(ctx, &transaction.TxRequest{
		To:   &s.incentivesContractAddress,
		Data: callData,
	})
	if err != nil {
		return nil, err
	}

	results, err := incentivesContractABI.Unpack("currentRoundAnchor", result)
	if err != nil {
		return nil, err
	}

	return results[0].([]byte), nil
}

// we dont need this for now
func (s *Service) checkIsWinning(ctx context.Context) (string, string, error) {
	callData, err := incentivesContractABI.Pack("currentWinnerSelectionAnchor")
	if err != nil {
		return "", "", err
	}

	call, err := s.txService.Call(ctx, &transaction.TxRequest{
		To:   &s.incentivesContractAddress,
		Data: callData,
	})
	if err != nil {
		return "", "", err
	}

	winnerAnchor, err := incentivesContractABI.Unpack("currentWinnerSelectionAnchor", call)
	if err != nil {
		return "", "", err
	}

	callData, err = incentivesContractABI.Pack("currentTruthSelectionAnchor")
	if err != nil {
		return "", "", err
	}

	call, err = s.txService.Call(ctx, &transaction.TxRequest{
		To:   &s.incentivesContractAddress,
		Data: callData,
	})
	if err != nil {
		return "", "", err
	}

	truthAnchor, err := incentivesContractABI.Unpack("currentTruthSelectionAnchor", call)
	if err != nil {
		return "", "", err
	}

	return winnerAnchor[0].(string), truthAnchor[0].(string), nil
}

func (s *Service) neighbourSelected(ctx context.Context) (bool, error) {
	callData, err := incentivesContractABI.Pack("isParticipatingInUpcomingRound", s.overlay)
	if err != nil {
		return false, err
	}

	result, err := s.txService.Call(ctx, &transaction.TxRequest{
		To:   &s.incentivesContractAddress,
		Data: callData,
	})
	if err != nil {
		return false, err
	}

	results, err := incentivesContractABI.Unpack("isParticipatingInUpcomingRound", result)
	if err != nil {
		return false, err
	}

	return results[0].(bool), nil
}

func parseABI(json string) abi.ABI {
	cabi, err := abi.JSON(strings.NewReader(json))
	if err != nil {
		panic(fmt.Sprintf("error creating ABI for redistribution contract: %v", err))
	}
	return cabi
}
