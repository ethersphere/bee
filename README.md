# Ethereum Swarm Bee

[![Go](https://github.com/ethersphere/bee/workflows/Go/badge.svg)](https://github.com/ethersphere/bee/actions)
[![GoDoc](https://godoc.org/github.com/ethersphere/bee?status.svg)](https://godoc.org/github.com/ethersphere/bee)

Bee is the second official Ethereum Swarm implementation. This project is in very early stage and under active development.

No compatibility with the first Ethereum Swarm implementation is provided, mainly because the change in underlaying protocol from devp2p to libp2p. A bee node cannot join swarm network and vice versa.

API compatibility will be guaranteed when version 1.0 is released, but not before that.

## Install

Prerequisites for installing bee from source are:

- Go - download the latest release from https://golang.org/dl
- git - download from https://git-scm.com/ or install with your package manager
- make

Installing from source by checking the git repository:

```sh
git clone https://github.com/ethersphere/bee
cd bee
make binary
cp dist/bee /usr/local/bin/bee
```

## Running the Bee node

Bee node provides CLI help that lists all available commands and flags. Every command has its own help.

```sh
bee -h
bee start -h
```

To run the node with the default configuration:

```sh
bee start
```

This command starts bee node including HTTP API for user interaction and P2P API for communication between bee nodes.

It will open an interactive prompt asking for a password which protects node private keys. Interactive prompt can be avoided by providing a CLI flag `--password-file` with path to the file that contains the password, just to pass it as the value to `--password` flag. These values are also possible to be set with environment variables or configuration file.

## Configuration

Configuration parameters can be passed to the bee node by:

- command line arguments
- environment variables
- configuration file

with that order of precedence.

Available command line parameters can be seen by invoking help flag `bee start -h`.

Environment variables are analogues to these flags and can be constructed with simple rules:

- remove `--` prefix from the flag name
- capitalize all characters in the flag name
- replace all `-` characters with `_`
- prepend `BEE_` prefix to the resulted string

For example, cli flag `--api-addr` has an analogues env variable `BEE_API_ADDR`

Configuration file path is by default `$HOME/.bee.yaml`, but it can be changed with `--config` cli flag or `BEE_CONFIG` environment variable.

Configuration file variables are all from the `bee start -h` help page, just without the `--` prefix. For example, `--api-addr` and `--data-dir` can be specified with configuration file such as this one:

```yaml
api-addr: 127.0.0.1:8085
data-dir: /data/bees/bee5
```

## File upload

File can be uploaded by making an HTTP request like this one:

```sh
curl -F file=@kitten.jpg http://localhost:8080/files
```

Curl will make a properly encoded `multipart/form-data` request sending the filename, content type and file content to the bee API, and it will return a response with the reference to the uploaded file:

```json
{"reference":"3b2791985f102fe645d1ebd7f51e522d277098fcd86526674755f762084b94ee"}
```

This reference is just an example, it will differ for every uploaded file.

To download a file, open an URL with that reference `http://localhost:8080/files/3b2791985f102fe645d1ebd7f51e522d277098fcd86526674755f762084b94ee`.

If you need to specify manually content type or filename during upload:

```sh
curl -H "Content-Type: image/x-jpeg" --data-binary @kitten.jpg localhost:8081/files?name=cat.jpg
```

The same response with file reference is returned.

To avoid uploading with command line tools, this HTML file can be opened in the browser and used to select and submit a file:

```html
<form action="http://localhost:8080/files" method="post" enctype="multipart/form-data">
 <div>
   <input type="file" name="file">
 </div>
 <div>
   <button>Upload</button>
 </div>
</form>
```

And download a file in the browser can be done by entering the URL `<API_address>/files/{reference}` in the address bar, where API_address is by default `http://localhost:8080` and reference is the reference value from the returned JSON response.

## Starting more bee nodes locally with debugging

It is possible to start multiple, persistent, completely independent, bee nodes on a single running operating system. This can be achieved with less complexity with prepared configuration files for every node.

An example configuration for the first node would be:

```yaml
api-addr: :8081
p2p-addr: :7071
debug-api-addr: 127.0.0.1:6061
enable-debug-api: true
data-dir: /tmp/bee/node1
password: some pass phze
verbosity: trace
tracing: true
```

Save a file named `node1.yaml` with this content, and create as many as you need by incrementing the number in in filename and `api-addr`, `p2p-addr`, `debug-api-addr` and `data-dir`.

For example, file `node2.yaml` should have this content:

```yaml
api-addr: :8082
p2p-addr: :7072
debug-api-addr: 127.0.0.1:6062
enable-debug-api: true
data-dir: /tmp/bee/node2
password: some pass phze
verbosity: trace
tracing: true
```

### Starting the first node

The first node address will be used for other nodes to discover themselves. It is usually referred as the `bootnode address`.

```sh
bee --config node1.yaml start
```

When the node starts it will print out some of its `p2p addresses` in a multiaddress form like this: `/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAm2LXfYsY9pXtgGdQ8oPb3bAkxwpfBE6AMzcscH1UkQLZM`. This address is the `bootnode address`.

### Starting other nodes

Other nodes should be started by providing the bootnode address to them, so that they can connect to each other:

```sh
bee --config node2.yaml start --bootnode /ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAm2LXfYsY9pXtgGdQ8oPb3bAkxwpfBE6AMzcscH1UkQLZM
bee --config node3.yaml start --bootnode /ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAm2LXfYsY9pXtgGdQ8oPb3bAkxwpfBE6AMzcscH1UkQLZM
bee --config node4.yaml start --bootnode /ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAm2LXfYsY9pXtgGdQ8oPb3bAkxwpfBE6AMzcscH1UkQLZM
...
```

## Getting node addresses

Every node can provide its overlay address and underlay addresses by reading the logs when the node starts or through Debug API. For example, for the node 1 started with configuration above:

```sh
curl localhost:6061/addresses
```

Debug API is not started by default and has to be explicitly enabled with `--enable-debug-api --debug-api-addr 127.0.0.1:6061` command line flags or options.

It will return a response with addresses:

```json
{
  "overlay": "7ecf777fdab7553fbc4db7f265a7bd80d27b171babe931bc61e9d7966974ef47",
  "underlay": [
    "/ip6/::1/udp/7071/quic/p2p/16Uiu2HAm2LXfYsY9pXtgGdQ8oPb3bAkxwpfBE6AMzcscH1UkQLZM",
    "/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAm2LXfYsY9pXtgGdQ8oPb3bAkxwpfBE6AMzcscH1UkQLZM",
    "/ip4/127.0.0.1/udp/7071/quic/p2p/16Uiu2HAm2LXfYsY9pXtgGdQ8oPb3bAkxwpfBE6AMzcscH1UkQLZM",
    "/ip6/::1/tcp/7071/p2p/16Uiu2HAm2LXfYsY9pXtgGdQ8oPb3bAkxwpfBE6AMzcscH1UkQLZM",
  ]
}
```

## Testing a connection with PingPong protocol

To check if two nodes are connected and to see the round trip time for message exchange between them, get the overlay address from one node, for example local node 2:

```sh
curl localhost:6062/addresses
```

Make sure that Debug API is enabled and address configured as in examples above.

And use that address in the Debug API call on another node, for example, local node 1:

```sh
curl -XPOST localhost:6061/pingpong/d4440baf2d79e481c3c6fd93a2014d2e6fe0386418829439f26d13a8253d04f1
```

## Generating protobuf

To process protocol buffer files and generate the Go code from it two tools are needed:

- protoc - https://github.com/protocolbuffers/protobuf/releases
- protoc-gen-gogofaster - https://github.com/gogo/protobuf

Makefile rule `protobuf` can be used to automate `protoc-gen-gogofaster` installation and code generation:

```sh
make protobuf
```

## Contributing

Please read the [coding guidelines](CODING.md).

## License

This library is distributed under the BSD-style license found in the [LICENSE](LICENSE) file.
