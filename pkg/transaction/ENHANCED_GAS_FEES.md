# Enhanced Gas Fee System for Bee Transaction Service

## Overview

This document describes the enhanced gas fee prediction and management system implemented in the Bee transaction service. The enhancements provide sophisticated fee calculation strategies, network congestion awareness, and prepare for the upcoming Ethereum Pectra upgrade.

## Key Features

### 1. Dynamic Fee Strategies

The system supports four different gas fee strategies:

- **Conservative**: Lower fees, longer confirmation times
- **Standard**: Uses network suggested fees (default Ethereum behavior)
- **Aggressive**: Higher fees for faster confirmation
- **Dynamic**: Automatically adapts to network conditions (recommended)

### 2. Network Congestion Awareness

The service continuously monitors network conditions including:
- Gas usage ratios across recent blocks
- Average base fees
- Priority fee trends
- Congestion-based fee adjustments

### 3. Pectra Upgrade Compatibility

Prepares for Ethereum's Pectra upgrade (March 2025) features:
- Blob fee calculations for increased capacity (3→6 blobs)
- Account abstraction preparation
- Enhanced L2 integration support

## Configuration Constants

```go
const (
    MaxGasFeeHistoryBlocks      = 20    // Blocks analyzed for fee history
    BaseFeePercentileTarget     = 50    // Target percentile for base fee
    PriorityFeePercentileTarget = 75    // Target percentile for priority fee
    NetworkCongestionThreshold  = 0.8   // Congestion detection threshold
    CongestionMultiplier        = 1.5   // Fee multiplier during congestion
    MinGasTipCap               = 1000000000  // 1 Gwei minimum tip
    MaxGasTipCap               = 50000000000 // 50 Gwei maximum tip
)
```

## Usage Examples

### Setting Fee Strategies

```go
// Set conservative strategy for cost savings
txService.SetFeeStrategy(StrategyConservative)

// Set aggressive strategy for urgent transactions
txService.SetFeeStrategy(StrategyAggressive)

// Set dynamic strategy for automatic adaptation (recommended)
txService.SetFeeStrategy(StrategyDynamic)
```

### Monitoring Network Conditions

```go
// Get current network metrics
congestion, baseFee, tipFee := txService.GetNetworkMetrics()

if congestion > 0.8 {
    // High congestion detected - consider waiting or using conservative strategy
    fmt.Printf("High network congestion: %.2f\n", congestion)
}
```

### Optimal Timing Prediction

```go
// Check if it's a good time to send a transaction
waitTime, err := txService.PredictOptimalGasTime(ctx)
if err != nil {
    return err
}

if waitTime > 0 {
    fmt.Printf("Suggest waiting %v for better gas prices\n", waitTime)
} else {
    fmt.Println("Optimal time to send transaction now")
}
```

### Blob Fee Calculation (Pectra)

```go
// Calculate blob fees for different blob counts
blobFee, err := txService.CalculateBlobFee(ctx, 6) // New Pectra maximum
if err != nil {
    return err
}
fmt.Printf("Blob fee for 6 blobs: %s wei\n", blobFee.String())
```

## Implementation Details

### Network Metrics Collection

The service maintains a `NetworkMetrics` structure that tracks:

```go
type NetworkMetrics struct {
    lastUpdate        time.Time
    avgBaseFee        *big.Int
    avgPriorityFee    *big.Int
    congestionRatio   float64
    blobBaseFee       *big.Int
    recentGasUsage    []float64
    mutex             sync.RWMutex
}
```

This data is updated every 30 seconds and used to make intelligent fee decisions.

### Dynamic Fee Calculation Algorithm

The dynamic strategy follows this logic:

1. **Analyze Network State**: Check recent block gas usage ratios
2. **Detect Congestion**: If usage > 80%, apply congestion multiplier
3. **Calculate Base Fees**: Use network suggestions with strategy adjustments
4. **Apply Bounds**: Ensure fees stay within reasonable limits
5. **Validate Coverage**: Ensure gasFeeCap covers baseFee + tip

### Strategy-Specific Behavior

| Strategy | Tip Multiplier | Use Case |
|----------|---------------|----------|
| Conservative | 80% of suggested | Cost optimization, non-urgent |
| Standard | 100% of suggested | Normal operations |
| Aggressive | 150% of suggested | Urgent transactions |
| Dynamic | Congestion-aware | Automatic adaptation |

## Pectra Upgrade Preparation

### Account Abstraction (EIP-7702)

The Pectra upgrade introduces account abstraction, enabling:
- Gas payments with ERC-20 tokens (USDC, DAI)
- Third-party gas sponsorship
- Transaction batching
- Gasless transactions for end users

### Enhanced Blob Capacity

Pectra doubles blob capacity from 3 to 6 blobs per block:
- Reduces L2 transaction costs
- Improves network throughput
- Changes blob fee dynamics

### Implementation Recommendations

1. **Prepare for Token Gas Payments**: Consider architecture changes to support ERC-20 gas payments
2. **Monitor Blob Fee Changes**: Track how increased capacity affects blob pricing
3. **Update Fee Strategies**: Network efficiency improvements may change optimal strategies
4. **Test Account Abstraction**: Prepare for smart account integrations

## Best Practices

### For Developers

1. **Use Dynamic Strategy**: Let the system adapt automatically to network conditions
2. **Monitor Metrics**: Check network congestion before sending important transactions
3. **Implement Retry Logic**: Use the prediction system to determine optimal retry timing
4. **Handle Bounds**: Be prepared for fee caps to prevent excessive costs

### For Node Operators

1. **Configure Thresholds**: Adjust congestion thresholds based on your use case
2. **Monitor Performance**: Track how different strategies affect confirmation times
3. **Plan for Pectra**: Prepare infrastructure for account abstraction features
4. **Update Regularly**: Keep up with network changes and strategy effectiveness

## Error Handling

The enhanced system includes robust error handling:

```go
// Graceful degradation to legacy calculation
if err := t.updateNetworkMetrics(ctx); err != nil {
    t.logger.Debug("failed to update network metrics", "error", err)
    // Falls back to standard fee calculation
}

// Bounds checking prevents excessive fees
if gasTipCap.Cmp(maxTip) > 0 {
    gasTipCap = maxTip
}
```

## Monitoring and Debugging

### Key Metrics to Monitor

- Network congestion ratio
- Average fee calculation times
- Transaction confirmation times by strategy
- Fee accuracy vs. actual network conditions

### Debug Logging

The system provides detailed debug logging:

```go
t.logger.Debug("calculated dynamic fees",
    "strategy", strategy,
    "congestion_ratio", congestionRatio,
    "gas_fee_cap", gasFeeCap,
    "gas_tip_cap", gasTipCap,
    "boost_percent", boostPercent)
```

## Migration Guide

### Upgrading from Legacy System

1. **Backward Compatibility**: Existing code continues to work unchanged
2. **Gradual Migration**: Start with `StrategyDynamic` for automatic optimization
3. **Monitor Results**: Compare transaction costs and confirmation times
4. **Fine-tune**: Adjust strategies based on your specific requirements

### Testing Recommendations

1. **Start with Testnet**: Test strategies on testnets before mainnet deployment
2. **Monitor Costs**: Track actual gas costs vs. predictions
3. **Performance Testing**: Test under various network conditions
4. **Failover Testing**: Ensure graceful degradation when metrics are unavailable

## Future Enhancements

### Planned Features

1. **Machine Learning Integration**: Predictive models for optimal timing
2. **Cross-Chain Fee Analysis**: Compare fees across different networks
3. **Advanced Retry Strategies**: Intelligent transaction replacement
4. **Fee Optimization Recommendations**: Automated strategy suggestions

### Pectra Integration Roadmap

1. **Phase 1 (Pre-Pectra)**: Enhanced fee prediction and blob calculations
2. **Phase 2 (Pectra Launch)**: Account abstraction integration
3. **Phase 3 (Post-Pectra)**: Full smart account support and token gas payments

## Conclusion

The enhanced gas fee system provides a robust foundation for intelligent transaction management. By combining network awareness, multiple strategies, and Pectra upgrade preparation, it ensures optimal transaction costs and confirmation times while maintaining backward compatibility.

The system is designed to adapt to the evolving Ethereum ecosystem, particularly the significant changes coming with the Pectra upgrade. Regular monitoring and strategy adjustment will help maintain optimal performance as network conditions change.