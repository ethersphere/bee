# Swarm Bee

This is an experiment to abstract libp2p as underlay networking for Ethereum Swarm.

Work in progress. This is by no means the final abstraction.

## Usage

Execute the commands in two terminals to start `node 1` and `node 2`:

```sh
go run ./cmd/bee start --listen :8501
```

```sh
go run ./cmd/bee start --listen :8502
```

Copy one of the multiaddresses from one running instance.

Make an HTTP request to `localhost:{PORT1}/pingpong/{MULTIADDRESS2}` like:

```sh
curl localhost:8501/pingpong/ip4/127.0.0.1/tcp/60304/p2p/Qmdao2FbfSK8ZcFxuUVmVDPUJifgRmbofNWH21WQESZm7x
```

Ping pong messages should be exchanged from `node 1` (listening on `PORT1`) to `node 2` (with multiaddress `MULTIADDRESS2`).

## Structure

- cmd/bee - a simple application integrating p2p and pingpong service
- pkg/api - a simple http api exposing pingpong endpoint
- pkg/p2p - p2p abstraction
- pkg/p2p/libp2p - p2p implementation using libp2p
- pkg/p2p/mock - p2p protocol testing tools
- pkg/p2p/protobuf - protobuf message encoding and decoding functions
- pkg/pingpong - p2p protocol implementation example

## Restrictions

- Package pkg/p2p only contains generalized abstraction of p2p protocol. It does not impose stream encoding.
- Package pkg/p2p/libp2p and is only allowed to depend on go-libp2p packages given that it is just one pkg/p2p implementation. No other implementation should depend on go-libp2p packages.
- Package pkg/p2p/protobuf provides all the helpers needed to make easer for protocol implementations to use protocol buffers for encoding.

## TODO

- P2P mock (protocol tester) implementation improvements
- Overlay addressing in libp2p (provide overlay address in p2p.Peer)
- Identity with private keys
- Figure out routing (whether to use libp2p Routing or to abstract hive on top of p2p package)
- Listener configurations (ipv4, ipv6, dns, tcp, ws, quic)
