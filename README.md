# Ethereum Swarm Bee

This is an experiment to abstract libp2p as underlay networking for Ethereum Swarm.

Work in progress. This is not the final abstraction.

## Install

```sh
make binary
cp dist/bee /usr/local/bin/bee
```

Docker image `janos/bee`.

## Usage (experimental api)

Execute the command terminals to start `node 1`:

```sh
bee start --api-addr :8501 --p2p-addr :30401
```

Use one of the multiaddresses as bootnode for `node 2` in order to connect them:

```sh
bee start --api-addr :8502 --p2p-addr :30402 --bootnode /ip4/127.0.0.1/tcp/30401/p2p/QmT4TNB4cKYanUjdYodw1Cns8cuVaRVo24hHNYcT7JjkTB
```

Use the last part of `node 1` multiaddress to ping it using `node 2` by making an HTTP request to `localhost:{PORT2}/pingpong/{ID1}` like:

```sh
curl localhost:8502/pingpong/QmT4TNB4cKYanUjdYodw1Cns8cuVaRVo24hHNYcT7JjkTB
```

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
- Instrumentation: logging, metrics, tracing, pprof...
