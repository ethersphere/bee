package libp2p

import (
	"net/netip"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	libp2prate "github.com/libp2p/go-libp2p/x/rate"
)

func newResourceManager() (network.ResourceManager, error) {
	// scaledDefaultLimits is a copy of the default limits, but with the system base limits increased
	// to handle the higher connection count of a bee node.
	scaledDefaultLimits := rcmgr.DefaultLimits

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
		return nil, err
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

	rm, err := rcmgr.NewResourceManager(limiter, rcmgr.WithTraceReporter(str), limitPerIp, rcmgr.WithConnRateLimiters(connLimiter))
	if err != nil {
		return nil, err
	}

	return rm, nil
}
