package streamcache

import (
	"context"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
	"resenje.org/singleflight"
)

type StreamCache struct {
	p2p.StreamerDisconnecter
	cache map[string]map[string]p2p.Stream //swarm.Address -> *streamName:streamObj
	sf    singleflight.Group
}

func New(disc p2p.StreamerDisconnecter) *StreamCache {
	return &StreamCache{
		cache: make(map[string]map[string]p2p.Stream),
	}
}

func (s *StreamCache) NetworkStatus() p2p.NetworkStatus {
	return s.StreamerDisconnecter.NetworkStatus()
}

func (s *StreamCache) Disconnect(overlay swarm.Address, reason string) error {
	// do we remove something from the cache here?
	return s.StreamerDisconnecter.Disconnect(overlay, reason)
}

func (s *StreamCache) Blocklist(overlay swarm.Address, duration time.Duration, reason string) error {
	// or here?
	return s.StreamerDisconnecter.Blocklist(overlay, duration, reason)
}

func (s *StreamCache) NewStream(ctx context.Context, address swarm.Address, h p2p.Headers, protocol, version, stream string) (p2p.Stream, error) {
	streamFullName := p2p.NewSwarmStreamName(protocol, version, stream)
	singleFlightKey := address.String() + streamFullName
	addressKey := address.String()

	res, _, err := s.sf.Do(ctx, singleFlightKey, func(ctx context.Context) (interface{}, error) {
		addressStreams, found := s.cache[addressKey]
		if !found {
			newStream, err := s.StreamerDisconnecter.NewStream(ctx, address, h, protocol, version, streamFullName)
			if err != nil {
				return nil, err
			}
			s.cache[addressKey] = map[string]p2p.Stream{
				streamFullName: newStream,
			}
			return newStream, nil
		}

		stream, found := addressStreams[streamFullName]
		if found {
			return stream, nil
		}

		newStream, err := s.StreamerDisconnecter.NewStream(ctx, address, h, protocol, version, streamFullName)
		if err != nil {
			return nil, err
		}

		s.cache[addressKey][streamFullName] = newStream
		return newStream, nil
	})

	return res.(p2p.Stream), err
}
