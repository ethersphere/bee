# Swarm Bee

[![Go](https://github.com/ethersphere/bee/workflows/Go/badge.svg)](https://github.com/ethersphere/bee/actions)
[![Go Reference](https://pkg.go.dev/badge/github.com/ethersphere/bee.svg)](https://pkg.go.dev/github.com/ethersphere/bee)
[![codecov](https://codecov.io/gh/ethersphere/bee/branch/master/graph/badge.svg?token=63RNRLO3RU)](https://codecov.io/gh/ethersphere/bee)
[![Go Report Card](https://goreportcard.com/badge/github.com/ethersphere/bee)](https://goreportcard.com/report/github.com/ethersphere/bee)
[![API OpenAPI Specs](https://img.shields.io/badge/openapi-api-blue)](https://docs.ethswarm.org/api/)
[![Debug API OpenAPI Specs](https://img.shields.io/badge/openapi-debugapi-lightblue)](https://docs.ethswarm.org/debug-api/)
![Docker Pulls](https://img.shields.io/docker/pulls/ethersphere/bee)
![GitHub all releases](https://img.shields.io/github/downloads/ethersphere/bee/total)
![GitHub](https://img.shields.io/github/license/ethersphere/bee)


## DISCLAIMER
This software is provided to you "as is", use at your own risk and without warranties of any kind.
It is your responsibility to read and understand how Swarm works and the implications of running this software.
The usage of Bee involves various risks, including, but not limited to:
damage to hardware or loss of funds associated with the Ethereum account connected to your node.
No developers or entity involved will be liable for any claims and damages associated with your use,
inability to use, or your interaction with other nodes or the software.

Our documentation is hosted at https://docs.ethswarm.org.

## Versioning

There are two versioning schemes used in Bee that you should be aware about. The main Bee version does **NOT** follow
strictly Semantic Versioning. As Bee includes number of protocols it would require bumping major version for every protocol updates. 
In this light minor versions can include breaking changes, but patch versions will be kept only for fixes.

Second set of versions that is important are the Bee's OpenAPI versions that define versioning of the Bee API and Bee Debug API. These versions **does follow** 
Semantic Versioning and hence you should follow these for breaking changes.

## Contributing

Please read the [coding guidelines](CODING.md).

## Installing

[Install instructions](https://docs.ethswarm.org/docs/installation/quick-start)

## Get in touch
[Only official website](https://www.ethswarm.org)


## License

This library is distributed under the BSD-style license found in the [LICENSE](LICENSE) file.

