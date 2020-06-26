# Ethereum Swarm Bee

[![Go](https://github.com/ethersphere/bee/workflows/Go/badge.svg)](https://github.com/ethersphere/bee/actions)
[![GoDoc](https://godoc.org/github.com/ethersphere/bee?status.svg)](https://godoc.org/github.com/ethersphere/bee)

```
Welcome to the Swarm.... Bzzz Bzzzz Bzzzz
                \     /                
            \    o ^ o    /            
              \ (     ) /              
   ____________(%%%%%%%)____________   
  (     /   /  )%%%%%%%(  \   \     )  
  (___/___/__/           \__\___\___)  
     (     /  /(%%%%%%%)\  \     )     
      (__/___/ (%%%%%%%) \___\__)      
              /(       )\              
            /   (%%%%%)   \            
                 (%%%)                 
                   !    
```


Our documenation is hosted at:
- https://swarm-gateways.net/bzz:/docs.swarm.eth/
- https://github.com/ethersphere/docs.github.io

## Contributing

Please read the [coding guidelines](CODING.md).

## Dev instructions
### Generating protobuf

To process protocol buffer files and generate the Go code from it two tools are needed:

- protoc - https://github.com/protocolbuffers/protobuf/releases
- protoc-gen-gogofaster - https://github.com/gogo/protobuf

Makefile rule `protobuf` can be used to automate `protoc-gen-gogofaster` installation and code generation:

```sh
make protobuf
```

## License

This library is distributed under the BSD-style license found in the [LICENSE](LICENSE) file.
