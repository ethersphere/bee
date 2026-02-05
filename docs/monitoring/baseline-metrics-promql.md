# Baseline Connection Metrics - PromQL Queries

Branch: `test-v28`

## Verified Metric Names

| Metric Name | Type | Description |
|-------------|------|-------------|
| `bee_libp2p_connection_dial_duration_seconds` | Histogram | Time to establish connection |
| `bee_libp2p_private_address_connections_total` | Counter | Connections via private addresses |
| `bee_libp2p_public_address_connections_total` | Counter | Connections via public addresses |
| `bee_libp2p_connection_address_attempts` | Histogram | Address attempts before success |
| `bee_libp2p_first_address_connection_count` | Counter | First-try successes |
| `bee_libp2p_later_address_connection_count` | Counter | Multi-attempt connections |

## PromQL Queries

### Dial Latency

```promql
# Average dial latency (seconds)
rate(bee_libp2p_connection_dial_duration_seconds_sum[5m]) / rate(bee_libp2p_connection_dial_duration_seconds_count[5m])

# 95th percentile dial latency
histogram_quantile(0.95, rate(bee_libp2p_connection_dial_duration_seconds_bucket[5m]))

# 99th percentile dial latency
histogram_quantile(0.99, rate(bee_libp2p_connection_dial_duration_seconds_bucket[5m]))
```

### Address Selection Efficiency

```promql
# First-attempt success rate (ideal = 1.0, higher is better)
bee_libp2p_first_address_connection_count / (bee_libp2p_first_address_connection_count + bee_libp2p_later_address_connection_count)

# Average addresses tried per connection (ideal = 1.0, lower is better)
rate(bee_libp2p_connection_address_attempts_sum[5m]) / rate(bee_libp2p_connection_address_attempts_count[5m])

# Connection attempts distribution
histogram_quantile(0.5, rate(bee_libp2p_connection_address_attempts_bucket[5m]))
```

### Private vs Public Address Usage

```promql
# Private address ratio (in cluster, higher = better internal routing)
bee_libp2p_private_address_connections_total / (bee_libp2p_private_address_connections_total + bee_libp2p_public_address_connections_total)

# Private connections rate
rate(bee_libp2p_private_address_connections_total[5m])

# Public connections rate
rate(bee_libp2p_public_address_connections_total[5m])
```

### Existing Kademlia Metrics

```promql
# Overall connection success rate
bee_kademlia_total_outbound_connections / bee_kademlia_total_outbound_connection_attempts

# Failed connection rate
rate(bee_kademlia_total_outbound_connection_failed_attempts[5m])

# Connection attempt rate
rate(bee_kademlia_total_outbound_connection_attempts[5m])
```

## Grafana Dashboard Panels

### Panel 1: Dial Latency Over Time
```promql
# Average
rate(bee_libp2p_connection_dial_duration_seconds_sum[5m]) / rate(bee_libp2p_connection_dial_duration_seconds_count[5m])

# P95
histogram_quantile(0.95, rate(bee_libp2p_connection_dial_duration_seconds_bucket[5m]))
```

### Panel 2: First-Try Success Rate (Gauge 0-100%)
```promql
100 * bee_libp2p_first_address_connection_count / (bee_libp2p_first_address_connection_count + bee_libp2p_later_address_connection_count)
```

### Panel 3: Address Type Distribution (Pie Chart)
```promql
bee_libp2p_private_address_connections_total
bee_libp2p_public_address_connections_total
```

### Panel 4: Connection Success Rate
```promql
100 * bee_kademlia_total_outbound_connections / bee_kademlia_total_outbound_connection_attempts
```

## Expected Changes After Optimization

| Metric | Baseline | After Optimization |
|--------|----------|-------------------|
| Dial latency (p95) | Measure | Should **decrease** |
| First-try success rate | Measure | Should **increase** |
| Avg attempts per connection | Measure | Should approach **1.0** |
| Private address ratio | Measure | Should **increase** (in cluster) |
