# Ethereum Swarm Bee

[![Go](https://github.com/ethersphere/bee/workflows/Go/badge.svg)](https://github.com/ethersphere/bee/actions)
[![GoDoc](https://godoc.org/github.com/ethersphere/bee?status.svg)](https://godoc.org/github.com/ethersphere/bee)

This is an experiment to abstract libp2p as underlay networking for Ethereum Swarm.

Work in progress. This is not the final abstraction.

## Install

```sh
make binary
cp dist/bee /usr/local/bin/bee
```

## Usage (experimental api)

Execute the command terminals to start `node 1`:

```sh
bee start --api-addr :8081 --p2p-addr :7071 --data-dir data1
```

### Bootnodes 
Use one of the multiaddresses as bootnode for `node 2` in order to connect them:

```sh
bee start --api-addr :8082 --p2p-addr :7072 --data-dir data2 --bootnode /ip4/127.0.0.1/tcp/30401/p2p/QmT4TNB4cKYanUjdYodw1Cns8cuVaRVo24hHNYcT7JjkTB
```

### Debugapi
Start `node 2` with debugapi enabled:

```sh
bee start --api-addr :8082 --p2p-addr :7072 --debug-api-addr :6062 --enable-debug-api --data-dir dist/storage2
```

Use one of the multiaddresses of `node 1` in order to connect them:

```sh
curl -XPOST localhost:6063/connect/ip4/127.0.0.1/tcp/30401/p2p/QmT4TNB4cKYanUjdYodw1Cns8cuVaRVo24hHNYcT7JjkTB
```

### Pingpong
Take the address of the connected peer to `node 1` from log line `peer "4932309428148935717" connected` and make an HTTP POST request to `localhost:{PORT1}/pingpong/{ADDRESS}` like:

```sh
curl -XPOST localhost:8502/pingpong/4932309428148935717
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

## Logging guidelines

Log messages are categorized in five different severities: error, warning, info, debug and trace, with semantic meaning associated with each of them.

Two types of application users are identified: regular user and developer. Regular user should not be presented with confusing technical implementation details in log messages, but only with meaningful information related to application operability in a form of meaningful statements. Developers are users that are aware of implementation details and they will benefit from technical details to help them debug the application problematic state.

This means that the same problematic event may have two log lines but with different severities. This is the case with Error/Debug or Warning/Debug combo where Error or Warning is meant for the regular user and the Debug for developers to help them investigate the issue. Info and Trace log levels are informative about the expected state changes, but Info, with operable information, for regular user, and Trace, with technical details, for developer.

### Error

Error log messages are meant for regular users to be informed about the event that is happening which is not expected or may require intervention from the user for application to function properly. Log messages should be written as a statement, e.g. *unable to connect to peer 12345* and should not reveal technical details about internal implementation, such are package name, variable name or internal variable state. It should contain information only relevant to the user operating the application, concepts that can be operated on using exposed application interfaces.

### Warning

Warning log messages are also meant for regular users, as Error log messages, but are just informative about the state of the application that it can handle itself, by retry mechanism, for example. They should have the same form as the Error log messages.

### Info

Info log messages are informing users about the changes that are changing the application state in the expected manner, or presenting the user the information that can be used to operate the application, such as *node address: 12345*.

### Debug

Debug log messages are meant for developers to identify the problem that has happened. They should contain as much as possible internal technical information and applications state to be able to reproduce and debug the issue. Format of such log messages should be declarative with colon `:` as the separator between code concepts as nested call stacks, components or abstractions, e.g. *handshake: parse peer /p2p/Qm2313123 address 12345: encoding/hex: odd length hex string*, just like Go error values are chained with annotations.

### Trace

Trace log messages are meant to inform developers about expected successful operations that are not relevant for regular users, but may be useful for understanding states in which application is going through. The form should be the same as Debug log messages.

## TODO

- P2P mock (protocol tester) implementation improvements
- Figure out routing (whether to use libp2p Routing or to abstract hive on top of p2p package)
- Instrumentation: tracing
