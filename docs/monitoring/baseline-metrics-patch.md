# Baseline Metrics Patch for Master

Apply these changes to master to collect baseline data before applying the optimization.

## 1. Add to `pkg/p2p/libp2p/metrics.go`

Add these fields to the `metrics` struct:

```go
// Connection baseline metrics (for A/B comparison)
ConnectionDialDuration      prometheus.Histogram
PrivateAddressConnections   prometheus.Counter
PublicAddressConnections    prometheus.Counter
ConnectionAddressAttempts   prometheus.Histogram
FirstAddressConnectionCount prometheus.Counter
LaterAddressConnectionCount prometheus.Counter
```

Add these to `newMetrics()`:

```go
ConnectionDialDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
    Namespace: m.Namespace,
    Subsystem: subsystem,
    Name:      "connection_dial_duration_seconds",
    Help:      "Time taken to establish a connection to a peer address.",
    Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
}),
PrivateAddressConnections: prometheus.NewCounter(prometheus.CounterOpts{
    Namespace: m.Namespace,
    Subsystem: subsystem,
    Name:      "private_address_connections_total",
    Help:      "Total connections established via private/internal addresses.",
}),
PublicAddressConnections: prometheus.NewCounter(prometheus.CounterOpts{
    Namespace: m.Namespace,
    Subsystem: subsystem,
    Name:      "public_address_connections_total",
    Help:      "Total connections established via public addresses.",
}),
ConnectionAddressAttempts: prometheus.NewHistogram(prometheus.HistogramOpts{
    Namespace: m.Namespace,
    Subsystem: subsystem,
    Name:      "connection_address_attempts",
    Help:      "Number of address attempts before successful connection.",
    Buckets:   []float64{1, 2, 3, 4, 5, 10},
}),
FirstAddressConnectionCount: prometheus.NewCounter(prometheus.CounterOpts{
    Namespace: m.Namespace,
    Subsystem: subsystem,
    Name:      "first_address_connection_count",
    Help:      "Connections that succeeded on the first address attempt.",
}),
LaterAddressConnectionCount: prometheus.NewCounter(prometheus.CounterOpts{
    Namespace: m.Namespace,
    Subsystem: subsystem,
    Name:      "later_address_connection_count",
    Help:      "Connections that required multiple address attempts.",
}),
```

## 2. Modify `pkg/p2p/libp2p/libp2p.go` Connect function

Replace the connection loop with instrumented version:

```go
func (s *Service) Connect(ctx context.Context, addrs []ma.Multiaddr) (address *bzz.Address, err error) {
    loggerV1 := s.logger.V(1).Register()

    defer func() {
        err = s.determineCurrentNetworkStatus(err)
    }()

    var info *libp2ppeer.AddrInfo
    var peerID libp2ppeer.ID
    var connectErr error
    var connectedAddr ma.Multiaddr
    skippedSelf := false
    attemptNumber := 0

    for _, addr := range addrs {
        attemptNumber++

        ai, err := libp2ppeer.AddrInfoFromP2pAddr(addr)
        if err != nil {
            return nil, fmt.Errorf("addr from p2p: %w", err)
        }

        info = ai
        peerID = ai.ID

        if peerID == s.host.ID() {
            s.logger.Debug("skipping connection to self", "peer_id", peerID, "underlay", info.Addrs)
            skippedSelf = true
            continue
        }

        hostAddr, err := buildHostAddress(info.ID)
        if err != nil {
            return nil, fmt.Errorf("build host address: %w", err)
        }

        remoteAddr := addr.Decapsulate(hostAddr)

        if overlay, found := s.peers.isConnected(info.ID, remoteAddr); found {
            address = &bzz.Address{
                Overlay:   overlay,
                Underlays: []ma.Multiaddr{addr},
            }
            return address, p2p.ErrAlreadyConnected
        }

        dialStart := time.Now()
        if err := s.connectionBreaker.Execute(func() error { return s.host.Connect(ctx, *info) }); err != nil {
            if errors.Is(err, breaker.ErrClosed) {
                s.metrics.ConnectBreakerCount.Inc()
                return nil, p2p.NewConnectionBackoffError(err, s.connectionBreaker.ClosedUntil())
            }
            s.logger.Warning("libp2p connect", "peer_id", peerID, "underlay", info.Addrs, "error", err)
            connectErr = err
            continue
        }
        dialDuration := time.Since(dialStart)

        // Connection succeeded - record metrics
        s.metrics.ConnectionDialDuration.Observe(dialDuration.Seconds())
        connectedAddr = addr
        connectErr = nil
        break
    }

    if connectErr != nil {
        return nil, fmt.Errorf("libp2p connect: %w", connectErr)
    }

    if skippedSelf {
        return nil, fmt.Errorf("cannot connect to self")
    }

    // Record connection metrics
    if connectedAddr != nil {
        s.metrics.ConnectionAddressAttempts.Observe(float64(attemptNumber))
        if attemptNumber == 1 {
            s.metrics.FirstAddressConnectionCount.Inc()
        } else {
            s.metrics.LaterAddressConnectionCount.Inc()
        }
        if manet.IsPrivateAddr(connectedAddr) {
            s.metrics.PrivateAddressConnections.Inc()
        } else {
            s.metrics.PublicAddressConnections.Inc()
        }
    }

    // ... rest of the function unchanged ...
```

## PromQL Queries for Baseline vs Optimized Comparison

### Before/After Queries

```promql
# Average dial latency (should DECREASE after optimization)
rate(bee_libp2p_connection_dial_duration_seconds_sum[5m]) / rate(bee_libp2p_connection_dial_duration_seconds_count[5m])

# 95th percentile dial latency
histogram_quantile(0.95, rate(bee_libp2p_connection_dial_duration_seconds_bucket[5m]))

# First-attempt success rate (should INCREASE after optimization)
bee_libp2p_first_address_connection_count / (bee_libp2p_first_address_connection_count + bee_libp2p_later_address_connection_count)

# Average addresses tried per connection (should DECREASE, closer to 1)
rate(bee_libp2p_connection_address_attempts_sum[5m]) / rate(bee_libp2p_connection_address_attempts_count[5m])

# Private address ratio (in cluster, should INCREASE after optimization)
bee_libp2p_private_address_connections_total / (bee_libp2p_private_address_connections_total + bee_libp2p_public_address_connections_total)

# Connection success rate (existing metric, should INCREASE)
bee_kademlia_total_outbound_connections / bee_kademlia_total_outbound_connection_attempts
```

### Grafana Dashboard Panels

1. **Dial Latency Over Time** - Line chart of average + p95
2. **First-Try Success Rate** - Gauge 0-100%
3. **Address Attempts Distribution** - Histogram
4. **Private vs Public Connections** - Pie chart
5. **Connection Success Rate** - Line chart
