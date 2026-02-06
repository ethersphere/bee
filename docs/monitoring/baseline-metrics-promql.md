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

## Label Filters

To scope all queries to a specific cluster and namespace, append the selector after each metric name:

```
{cluster="swarm1", namespace="bee-light-testnet"}
```

Example: `bee_libp2p_connection_dial_duration_seconds_sum` becomes  
`bee_libp2p_connection_dial_duration_seconds_sum{cluster="swarm1", namespace="bee-light-testnet"}`.

Apply the same `{...}` to every metric in the query (each `rate(...)`, histogram, and counter).

**Example with filters (Dial Latency):**
```promql
rate(bee_libp2p_connection_dial_duration_seconds_sum{cluster="swarm1", namespace="bee-light-testnet"}[5m]) / rate(bee_libp2p_connection_dial_duration_seconds_count{cluster="swarm1", namespace="bee-light-testnet"}[5m])
```

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

## Grafana Dashboard: Single Dashboard Setup

**Dashboard name:** `Bee libp2p Connection Baseline`

Use the label filter `{cluster="swarm1", namespace="bee-light-testnet"}` (or your target) on every metric in the queries below when creating panels.

---

### How to create the dashboard

1. In Grafana: **Dashboards** → **New** → **New dashboard**.
2. Set the dashboard title: click the dashboard settings (gear) → **General** → **Name**: `Bee libp2p Connection Baseline` → **Save dashboard** (choose folder and save).
3. Add panels one by one: **Add** → **Visualization** (or **Add panel** on an empty dashboard). For each panel:
   - Choose **Prometheus** as data source.
   - Set **Panel title** (see panel names below).
   - Set **Visualization** type (see below).
   - Paste the PromQL into the query editor; add your label filter to each metric if needed (e.g. `metric_name{cluster="swarm1", namespace="bee-light-testnet"}`).
   - Apply **Apply** or **Save**.
4. Arrange panels in a grid (drag/resize), then **Save dashboard** (top right).

---

### Panel 1: Dial Latency (Avg, P95, P99)

| Setting | Value |
|--------|--------|
| **Panel title** | `Dial Latency (Avg, P95, P99)` |
| **Visualization** | Time series |

**Query A (Average):**
```promql
rate(bee_libp2p_connection_dial_duration_seconds_sum{cluster="swarm1", namespace="bee-light-testnet"}[5m]) / rate(bee_libp2p_connection_dial_duration_seconds_count{cluster="swarm1", namespace="bee-light-testnet"}[5m])
```
- **Legend:** `Avg`

**Query B (P95):**
```promql
histogram_quantile(0.95, rate(bee_libp2p_connection_dial_duration_seconds_bucket{cluster="swarm1", namespace="bee-light-testnet"}[5m]))
```
- **Legend:** `P95`

**Query C (P99):**
```promql
histogram_quantile(0.99, rate(bee_libp2p_connection_dial_duration_seconds_bucket{cluster="swarm1", namespace="bee-light-testnet"}[5m]))
```
- **Legend:** `P99`

**Options:** Unit **seconds (s)**; add all three as separate queries in the same panel.

---

### Panel 2: First-Try Address Success Rate

| Setting | Value |
|--------|--------|
| **Panel title** | `First-Try Address Success Rate` |
| **Visualization** | Gauge |

**Query:**
```promql
100 * bee_libp2p_first_address_connection_count{cluster="swarm1", namespace="bee-light-testnet"} / (bee_libp2p_first_address_connection_count{cluster="swarm1", namespace="bee-light-testnet"} + bee_libp2p_later_address_connection_count{cluster="swarm1", namespace="bee-light-testnet"})
```

**Options:** Min **0**, Max **100**; Unit **percent (0–100)**; threshold bands optional (e.g. green ≥ 80, yellow ≥ 50, red &lt; 50).

---

### Panel 3: Kademlia Connection Success Rate

| Setting | Value |
|--------|--------|
| **Panel title** | `Kademlia Connection Success Rate` |
| **Visualization** | Gauge |

**Query:**
```promql
100 * bee_kademlia_total_outbound_connections{cluster="swarm1", namespace="bee-light-testnet"} / bee_kademlia_total_outbound_connection_attempts{cluster="swarm1", namespace="bee-light-testnet"}
```

**Options:** Min **0**, Max **100**; Unit **percent (0–100)**.

---

### Panel 4: Address Type Distribution (Private vs Public)

| Setting | Value |
|--------|--------|
| **Panel title** | `Address Type Distribution (Private vs Public)` |
| **Visualization** | Pie chart |

**Query A:**
```promql
bee_libp2p_private_address_connections_total{cluster="swarm1", namespace="bee-light-testnet"}
```
- **Legend:** `Private`

**Query B:**
```promql
bee_libp2p_public_address_connections_total{cluster="swarm1", namespace="bee-light-testnet"}
```
- **Legend:** `Public`

**Options:** Two queries in the same panel; "Pie chart" or "Donut" as preferred.

---

### Panel 5: Private vs Public Connection Rate Over Time

| Setting | Value |
|--------|--------|
| **Panel title** | `Private vs Public Connection Rate` |
| **Visualization** | Time series |

**Query A:**
```promql
rate(bee_libp2p_private_address_connections_total{cluster="swarm1", namespace="bee-light-testnet"}[5m])
```
- **Legend:** `Private`

**Query B:**
```promql
rate(bee_libp2p_public_address_connections_total{cluster="swarm1", namespace="bee-light-testnet"}[5m])
```
- **Legend:** `Public`

**Options:** Unit **short** (ops/s) or **reqps**; stack or lines as preferred.

---

### Panel 6: Avg Addresses Tried per Connection

| Setting | Value |
|--------|--------|
| **Panel title** | `Avg Addresses Tried per Connection` |
| **Visualization** | Time series (or Stat for single value) |

**Query:**
```promql
rate(bee_libp2p_connection_address_attempts_sum{cluster="swarm1", namespace="bee-light-testnet"}[5m]) / rate(bee_libp2p_connection_address_attempts_count{cluster="swarm1", namespace="bee-light-testnet"}[5m])
```

**Options:** Ideal near **1.0**; no unit or "short" for a dimensionless ratio.

---

### Panel 7: Kademlia Connection Attempt & Fail Rate

| Setting | Value |
|--------|--------|
| **Panel title** | `Kademlia Connection Attempt & Fail Rate` |
| **Visualization** | Time series |

**Query A (Attempts):**
```promql
rate(bee_kademlia_total_outbound_connection_attempts{cluster="swarm1", namespace="bee-light-testnet"}[5m])
```
- **Legend:** `Attempts`

**Query B (Failed):**
```promql
rate(bee_kademlia_total_outbound_connection_failed_attempts{cluster="swarm1", namespace="bee-light-testnet"}[5m])
```
- **Legend:** `Failed`

**Options:** Unit **short** (ops/s); useful to see attempt volume and failure rate over time.

---

### Suggested dashboard layout

| Row | Panels |
|-----|--------|
| Row 1 | **Dial Latency (Avg, P95, P99)** (full width) |
| Row 2 | **First-Try Address Success Rate** \| **Kademlia Connection Success Rate** (two gauges side by side) |
| Row 3 | **Address Type Distribution (Private vs Public)** \| **Avg Addresses Tried per Connection** |
| Row 4 | **Private vs Public Connection Rate** \| **Kademlia Connection Attempt & Fail Rate** |

Resize and reorder as needed; save the dashboard when done.

## Expected Changes After Optimization

| Metric | Baseline | After Optimization |
|--------|----------|-------------------|
| Dial latency (p95) | Measure | Should **decrease** |
| First-try success rate | Measure | Should **increase** |
| Avg attempts per connection | Measure | Should approach **1.0** |
| Private address ratio | Measure | Should **increase** (in cluster) |
