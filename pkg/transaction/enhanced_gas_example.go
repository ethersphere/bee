// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/log"
)

// EnhancedGasExample demonstrates the usage of enhanced gas fee prediction
// and Pectra upgrade compatibility features.
//
// The enhanced gas fee system provides:
// 1. Dynamic fee calculation based on network congestion
// 2. Multiple fee strategies (Conservative, Standard, Aggressive, Dynamic)
// 3. Network congestion awareness
// 4. Blob fee calculation for Pectra upgrade
// 5. Optimal timing predictions
func EnhancedGasExample(txService Service, logger log.Logger) {
	ctx := context.Background()

	// Example 1: Setting different fee strategies
	logger.Info("=== Fee Strategy Examples ===")

	// Conservative strategy - Lower fees, slower confirmation
	txService.SetFeeStrategy(StrategyConservative)
	logger.Info("Set to Conservative strategy - Lower fees, may take longer to confirm")

	// Standard strategy - Default Ethereum network suggestions
	txService.SetFeeStrategy(StrategyStandard)
	logger.Info("Set to Standard strategy - Uses network suggested fees")

	// Aggressive strategy - Higher fees, faster confirmation
	txService.SetFeeStrategy(StrategyAggressive)
	logger.Info("Set to Aggressive strategy - Higher fees for faster confirmation")

	// Dynamic strategy - Adapts to network conditions (recommended)
	txService.SetFeeStrategy(StrategyDynamic)
	logger.Info("Set to Dynamic strategy - Adapts based on network congestion")

	// Example 2: Monitor network metrics
	logger.Info("=== Network Metrics ===")
	congestion, baseFee, tipFee := txService.GetNetworkMetrics()
	logger.Info("current network status",
		"congestion_ratio", fmt.Sprintf("%.2f", congestion),
		"avg_base_fee", baseFee.String(),
		"avg_tip_fee", tipFee.String())

	if congestion > 0.8 {
		logger.Info("high network congestion detected - transactions may be expensive")
	} else if congestion > 0.5 {
		logger.Info("moderate network congestion")
	} else {
		logger.Info("low network congestion - good time for transactions")
	}

	// Example 3: Predict optimal gas timing
	logger.Info("=== Optimal Timing Prediction ===")
	optimalWait, err := txService.PredictOptimalGasTime(ctx)
	if err != nil {
		logger.Error(err, "failed to predict optimal timing")
	} else if optimalWait > 0 {
		logger.Info("suggest waiting for better gas prices", "wait_duration", optimalWait)
	} else {
		logger.Info("optimal time to send transaction now")
	}

	// Example 4: Blob fee calculation for Pectra upgrade
	logger.Info("=== Pectra Blob Fee Calculation ===")

	// Calculate blob fees for different blob counts
	blobCounts := []int{1, 3, 6} // 6 is the new Pectra maximum
	for _, count := range blobCounts {
		blobFee, err := txService.CalculateBlobFee(ctx, count)
		if err != nil {
			logger.Error(err, "failed to calculate blob fee", "blob_count", count)
			continue
		}
		logger.Info("blob fee calculated",
			"blob_count", count,
			"blob_fee_wei", blobFee.String(),
			"blob_fee_gwei", new(big.Int).Div(blobFee, big.NewInt(1e9)).String())
	}

	// Example 5: Smart transaction sending with dynamic fees
	logger.Info("=== Smart Transaction Example ===")

	// Create a sample transaction request
	recipient := common.HexToAddress("0x1234567890123456789012345678901234567890")
	txRequest := &TxRequest{
		To:          &recipient,
		Value:       big.NewInt(1000000000000000000), // 1 ETH
		Data:        []byte{},
		Description: "Enhanced gas fee example transaction",
	}

	// Check if it's a good time to send
	waitTime, err := txService.PredictOptimalGasTime(ctx)
	if err == nil && waitTime > 0 {
		logger.Info("waiting for optimal gas conditions", "wait_time", waitTime)
		// In a real application, you might want to wait or schedule the transaction
	}

	// Send transaction with dynamic fee calculation
	// The service will automatically use the configured strategy and network conditions
	txHash, err := txService.Send(ctx, txRequest, 0) // 0% additional boost, let dynamic calculation handle it
	if err != nil {
		logger.Error(err, "failed to send transaction")
		return
	}

	logger.Info("transaction sent with enhanced gas calculation", "tx_hash", txHash.Hex())

	// Example 6: Monitor transaction and provide feedback
	receipt, err := txService.WaitForReceipt(ctx, txHash)
	if err != nil {
		logger.Error(err, "failed to get transaction receipt")
		return
	}

	// Calculate actual fee paid
	actualFee := new(big.Int).Mul(big.NewInt(int64(receipt.GasUsed)), receipt.EffectiveGasPrice)
	logger.Info("transaction confirmed",
		"block_number", receipt.BlockNumber,
		"gas_used", receipt.GasUsed,
		"effective_gas_price", receipt.EffectiveGasPrice.String(),
		"total_fee_wei", actualFee.String(),
		"total_fee_eth", new(big.Int).Div(actualFee, big.NewInt(1e18)).String())
}

// NetworkCongestionAnalyzer provides utilities for analyzing network conditions
type NetworkCongestionAnalyzer struct {
	service Service
	logger  log.Logger
}

// NewNetworkCongestionAnalyzer creates a new network congestion analyzer
func NewNetworkCongestionAnalyzer(service Service, logger log.Logger) *NetworkCongestionAnalyzer {
	return &NetworkCongestionAnalyzer{
		service: service,
		logger:  logger,
	}
}

// AnalyzeAndRecommend analyzes current network conditions and recommends actions
func (n *NetworkCongestionAnalyzer) AnalyzeAndRecommend(ctx context.Context) (*NetworkRecommendation, error) {
	congestion, baseFee, tipFee := n.service.GetNetworkMetrics()
	optimalWait, err := n.service.PredictOptimalGasTime(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to predict optimal timing: %w", err)
	}

	recommendation := &NetworkRecommendation{
		CongestionRatio: congestion,
		BaseFee:         baseFee,
		TipFee:          tipFee,
		OptimalWait:     optimalWait,
	}

	// Determine recommended strategy
	if congestion > 0.9 {
		recommendation.Strategy = StrategyConservative
		recommendation.Reason = "Very high congestion - use conservative strategy to save costs"
	} else if congestion > 0.7 {
		recommendation.Strategy = StrategyStandard
		recommendation.Reason = "High congestion - use standard strategy"
	} else if congestion < 0.3 {
		recommendation.Strategy = StrategyAggressive
		recommendation.Reason = "Low congestion - safe to use aggressive strategy for faster confirmation"
	} else {
		recommendation.Strategy = StrategyDynamic
		recommendation.Reason = "Moderate congestion - dynamic strategy will adapt automatically"
	}

	n.logger.Info("network analysis complete",
		"congestion", fmt.Sprintf("%.2f", congestion),
		"recommended_strategy", recommendation.Strategy,
		"reason", recommendation.Reason)

	return recommendation, nil
}

// NetworkRecommendation contains analysis results and recommendations
type NetworkRecommendation struct {
	CongestionRatio float64
	BaseFee         *big.Int
	TipFee          *big.Int
	OptimalWait     time.Duration
	Strategy        GasFeeStrategy
	Reason          string
}

// PectraUpgradeGuide provides guidance for Pectra upgrade compatibility
type PectraUpgradeGuide struct {
	service Service
	logger  log.Logger
}

// NewPectraUpgradeGuide creates a new Pectra upgrade guide
func NewPectraUpgradeGuide(service Service, logger log.Logger) *PectraUpgradeGuide {
	return &PectraUpgradeGuide{
		service: service,
		logger:  logger,
	}
}

// DemonstratePectraFeatures shows how to use Pectra-specific features
func (p *PectraUpgradeGuide) DemonstratePectraFeatures(ctx context.Context) error {
	p.logger.Info("=== Pectra Upgrade Features Demo ===")

	// Feature 1: Enhanced blob capacity (3 -> 6 blobs)
	p.logger.Info("Pectra doubles blob capacity from 3 to 6 blobs per block")
	p.logger.Info("This reduces L2 transaction costs and improves throughput")

	// Calculate fees for both old and new blob limits
	oldMaxBlobs := 3
	newMaxBlobs := 6

	oldBlobFee, err := p.service.CalculateBlobFee(ctx, oldMaxBlobs)
	if err != nil {
		return fmt.Errorf("failed to calculate old blob fee: %w", err)
	}

	newBlobFee, err := p.service.CalculateBlobFee(ctx, newMaxBlobs)
	if err != nil {
		return fmt.Errorf("failed to calculate new blob fee: %w", err)
	}

	p.logger.Info("blob fee comparison",
		"old_max_blobs", oldMaxBlobs,
		"old_blob_fee", oldBlobFee.String(),
		"new_max_blobs", newMaxBlobs,
		"new_blob_fee", newBlobFee.String())

	// Feature 2: Account Abstraction implications
	p.logger.Info("Pectra introduces account abstraction (EIP-7702)")
	p.logger.Info("Future versions may support:")
	p.logger.Info("- Gas payments with ERC-20 tokens (USDC, DAI)")
	p.logger.Info("- Third-party gas sponsorship")
	p.logger.Info("- Transaction batching")
	p.logger.Info("- Gasless transactions for users")

	// Feature 3: Validator efficiency improvements
	p.logger.Info("Pectra validator improvements:")
	p.logger.Info("- Maximum effective balance increased from 32 ETH to 2048 ETH")
	p.logger.Info("- Reduced network overhead from validator aggregation")
	p.logger.Info("- More efficient staking operations")

	// Feature 4: Recommendations for developers
	p.logger.Info("Recommendations for Pectra compatibility:")
	p.logger.Info("1. Use dynamic fee strategies to adapt to improved network efficiency")
	p.logger.Info("2. Prepare for blob fee changes in L2 integrations")
	p.logger.Info("3. Consider implementing account abstraction support")
	p.logger.Info("4. Monitor network metrics as congestion patterns may change")

	return nil
}

// Usage example function
func ExampleUsage() {
	// This example shows how to integrate the enhanced gas features
	// into your application

	/*
		// Initialize your transaction service (this would be your actual service)
		txService, err := NewService(logger, overlayEthAddress, backend, signer, store, chainID, monitor)
		if err != nil {
			log.Fatal(err)
		}
		defer txService.Close()

		// Run the enhanced gas examples
		EnhancedGasExample(txService, logger)

		// Analyze network conditions
		analyzer := NewNetworkCongestionAnalyzer(txService, logger)
		recommendation, err := analyzer.AnalyzeAndRecommend(context.Background())
		if err != nil {
			logger.Error(err, "failed to analyze network")
		} else {
			// Apply the recommended strategy
			txService.SetFeeStrategy(recommendation.Strategy)
			logger.Info("applied recommended gas strategy", "strategy", recommendation.Strategy)
		}

		// Demonstrate Pectra features
		pectraGuide := NewPectraUpgradeGuide(txService, logger)
		err = pectraGuide.DemonstratePectraFeatures(context.Background())
		if err != nil {
			logger.Error(err, "failed to demonstrate Pectra features")
		}
	*/
}
