package libp2p

import (
	"net/netip"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	libp2prate "github.com/libp2p/go-libp2p/x/rate"
	ma "github.com/multiformats/go-multiaddr"
)

func newResourceManager(bootnodes []string, allowPrivateCIDRs bool) (network.ResourceManager, int, error) {
	// scaledDefaultLimits is a copy of the default limits, but with the system base limits increased
	// to handle the higher connection count of a bee node.
	scaledDefaultLimits := rcmgr.DefaultLimits

	// Example: Protocol Prioritization
	// This ensures that "Background" protocols don't consume all resources, leaving room for "Critical" ones.
	// Note: protocol.ID strings must match your actual protocol IDs (e.g., "/hive/1.0.0", "/swap/1.0.0").
	/*
		cfg := rcmgr.PartialLimitConfig{
			Protocols: map[protocol.ID]rcmgr.ResourceLimits{
				"/hive/1.0.0": { // Critical Topology Protocol
					Streams:         rcmgr.Unlimited, // Give it priority
					StreamsInbound:  rcmgr.Unlimited,
					StreamsOutbound: rcmgr.Unlimited,
				},
				"/light/1.0.0": { // Less Critical Background Protocol
					Streams:         100, // Cap it to reserve space for others
					StreamsInbound:  50,
					StreamsOutbound: 50,
				},
			},
		}
		// Then apply it: limits := cfg.Build(scaledDefaultLimits.AutoScale())
	*/

	// Base limits (for low-memory devices like Raspberry Pi 1GB)
	scaledDefaultLimits.SystemBaseLimit.ConnsInbound = 512
	scaledDefaultLimits.SystemBaseLimit.ConnsOutbound = 512
	scaledDefaultLimits.SystemBaseLimit.Conns = 512
	scaledDefaultLimits.SystemBaseLimit.FD = 1024
	scaledDefaultLimits.SystemBaseLimit.Memory = 128 << 20 // 128 MB
	scaledDefaultLimits.SystemBaseLimit.Streams = 5120
	scaledDefaultLimits.SystemBaseLimit.StreamsInbound = 5120
	scaledDefaultLimits.SystemBaseLimit.StreamsOutbound = 5120

	// Scaling limits (for high-memory servers)
	// These values are added for every 1GB of *allowed* memory (which is typically 1/8th of system RAM)
	// Example: 16GB System RAM -> 2GB Allowed Mem -> Adds 2x these values to the base
	scaledDefaultLimits.SystemLimitIncrease.ConnsInbound = 2048
	scaledDefaultLimits.SystemLimitIncrease.ConnsOutbound = 2048
	scaledDefaultLimits.SystemLimitIncrease.Conns = 2048
	scaledDefaultLimits.SystemLimitIncrease.Memory = 1 << 30 // 1 GB
	scaledDefaultLimits.SystemLimitIncrease.Streams = 20480
	scaledDefaultLimits.SystemLimitIncrease.StreamsInbound = 20480
	scaledDefaultLimits.SystemLimitIncrease.StreamsOutbound = 20480

	// Boost transient limits for faster DHT lookups (Kademlia requires frequent short-lived connections)
	scaledDefaultLimits.TransientBaseLimit.ConnsInbound = 256
	scaledDefaultLimits.TransientBaseLimit.ConnsOutbound = 512

	// Create our limits by using our cfg and replacing the default values with values from `scaledDefaultLimits`
	limits := scaledDefaultLimits.AutoScale()

	// The resource manager expects a limiter, so we create one from our limits.
	limiter := rcmgr.NewFixedLimiter(limits)

	// Calculate IP limits dynamically based on the total system limits.
	// We allow 25% of the total system connections to come from a single IP.
	// This protects small nodes (e.g. 512 total -> 128 per IP) while allowing
	// "whales" (NATs) on larger servers (e.g. 8192 total -> 2048 per IP).
	totalConns := limiter.GetSystemLimits().GetConnTotalLimit()
	ipLimit := max(totalConns/4, 32) // Absolute minimum to allow basic peering

	limitPerIp := rcmgr.WithLimitPerSubnet(
		[]rcmgr.ConnLimitPerSubnet{{PrefixLength: 32, ConnCount: ipLimit}}, // IPv4 /32 (Single IP)
		[]rcmgr.ConnLimitPerSubnet{{PrefixLength: 56, ConnCount: ipLimit}}, // IPv6 /56 subnet
	)

	str, err := rcmgr.NewStatsTraceReporter()
	if err != nil {
		return nil, 0, err
	}

	// Custom rate limiter for connection attempts
	// 20 peers cluster adaptation:
	// Allow bursts of connection attempts (e.g. restart) but prevent DDOS.
	connLimiter := &libp2prate.Limiter{
		// Allow unlimited local connections (same as default)
		NetworkPrefixLimits: []libp2prate.PrefixLimit{
			{Prefix: netip.MustParsePrefix("127.0.0.0/8"), Limit: libp2prate.Limit{}},
			{Prefix: netip.MustParsePrefix("::1/128"), Limit: libp2prate.Limit{}},
		},
		GlobalLimit: libp2prate.Limit{}, // Unlimited global
		SubnetRateLimiter: libp2prate.SubnetLimiter{
			IPv4SubnetLimits: []libp2prate.SubnetLimit{
				{
					PrefixLength: 32, // Per IP
					// Allow 10 connection attempts per second per IP, burst up to 40
					Limit: libp2prate.Limit{RPS: 10.0, Burst: 40},
				},
			},
			IPv6SubnetLimits: []libp2prate.SubnetLimit{
				{
					PrefixLength: 56, // Per Subnet
					Limit:        libp2prate.Limit{RPS: 10.0, Burst: 40},
				},
			},
			GracePeriod: 10 * time.Second,
		},
	}

	var opts []rcmgr.Option
	opts = append(opts, rcmgr.WithTraceReporter(str))
	opts = append(opts, limitPerIp)
	opts = append(opts, rcmgr.WithConnRateLimiters(connLimiter))

	var allowlistedMultiaddrs []ma.Multiaddr

	if len(bootnodes) > 0 {
		for _, a := range bootnodes {
			m, err := ma.NewMultiaddr(a)
			if err != nil {
				// skip invalid bootnode addresses
				continue
			}
			allowlistedMultiaddrs = append(allowlistedMultiaddrs, m)
		}
	}

	if allowPrivateCIDRs {
		// Add standard private IP ranges to allowlist
		// This ensures that nodes on private networks (10.x, 192.168.x etc)
		// are not rate-limited or blocked, allowing local cluster setups.
		privateRanges := []string{
			"/ip4/10.0.0.0/ipcidr/8",
			"/ip4/172.16.0.0/ipcidr/12",
			"/ip4/192.168.0.0/ipcidr/16",
			"/ip6/fc00::/ipcidr/7",
		}
		for _, r := range privateRanges {
			m, err := ma.NewMultiaddr(r)
			if err == nil {
				allowlistedMultiaddrs = append(allowlistedMultiaddrs, m)
			}
		}
	}

	if len(allowlistedMultiaddrs) > 0 {
		opts = append(opts, rcmgr.WithAllowlistedMultiaddrs(allowlistedMultiaddrs))
	}

	rm, err := rcmgr.NewResourceManager(limiter, opts...)
	if err != nil {
		return nil, 0, err
	}

	return rm, totalConns, nil
}
