# Swarm Bee

This is an experiment to abstract libp2p as underlay networking for Ethereum Swarm.

Work in progress. This is by no means the final abstraction.

## Usage

In one terminal:

```sh
go run ./cmd/bee
```

Copy one of the multiaddresses.

In another terminal


```sh
go run ./cmd/bee -target COPIED_ADDRESS
```

Ping pong messages should be exchanged.

## Structure

- cmd/bee - a simple application integrating p2p and pingpong service
- pkg/p2p - p2p abstraction
- pkg/p2p/libp2p - p2p implementation using libp2p
- pkg/p2p/protobuf - protobuf message encoding and decoding functions
- pkg/pingpong - p2p protocol implementation example

## Restrictions

- Package pkg/p2p only contains generalized abstraction of p2p protocol. It does not impose stream encoding.
- Package pkg/p2p/libp2p and is only allowed to depend on go-libp2p packages given that it is just one pkg/p2p implementation. No other implementation should depend on go-libp2p packages.
- Package pkg/p2p/protobuf provides all the helpers needed to make easer for protocol implementations to use protocol buffers for encoding.

## TODO

- Mock testing for pingpong service as the example
- P2P mock (protocol tester) implementation
- Identity with private keys
- Figure out routing (whether to use libp2p Routing or to abstract hive on top of p2p package)
- Listener configurations (ipv4, ipv6, dns, tcp, ws, quic)
