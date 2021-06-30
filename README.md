# Swarm Bee

[![Go](https://github.com/ethersphere/bee/workflows/Go/badge.svg)](https://github.com/ethersphere/bee/actions)
[![Go Reference](https://pkg.go.dev/badge/github.com/ethersphere/bee.svg)](https://pkg.go.dev/github.com/ethersphere/bee)
[![Coverage Status](https://coveralls.io/repos/github/ethersphere/bee/badge.svg)](https://coveralls.io/github/ethersphere/bee)


## DISCLAIMER
This software is provided to you "as is", use at your own risk and without warranties of any kind.
It is your responsibility to read and understand how Swarm works and the implications of running this software.
The usage of Bee involves various risks, including, but not limited to:
damage to hardware or loss of funds associated with the Ethereum account connected to your node.
No developers or entity involved will be liable for any claims and damages associated with your use,
inability to use, or your interaction with other nodes or the software.

Our documentation is hosted at https://docs.ethswarm.org.

## Contributing

Please read the [coding guidelines](CODING.md).

## Installing

[Install instructions](https://docs.ethswarm.org/docs/installation/quick-start)

## Install Bee
The swarm thrives on decentralisation, and Bee is designed so that it works best when many individuals contribute to the health and distributed nature of the system by each running a Bee node.

It is easy to set up Bee on small and inexpensive computers, such as a Raspberry Pi 4, spare hardware you have lying around, or even a cheap cloud hosted VPS (we recommend small, independent providers and colocations).

-Installing Bee
Bee is packaged for MacOS and Ubuntu, Raspbian, Debian and CentOS based Linux distributions.

If your system is not supported, please see the manual installation section for information on how to install Bee.

-INFO
If you would like to run a hive of many Bees, checkout the node hive operators section for information on how to operate and monitor many Bees at once.

To install Bee you will need to go through the following process.

Set up the external signer for Bee, Bee Clef. (Recommended)
Install Bee and set it up to run as a service.
Configure Bee.
Fund your node with XDAI and BZZ
Wait for your chequebook transactions to complete and batch store to update.
Check Bee is working.
Install Bee Clef#
Bee makes use of Go Ethereum's external signer, Clef.

Because Bee must sign a lot of transactions automatically and quickly, a Bee specific version of Clef, bee-clef has been packaged which includes all the relevant configuration and implements the specific configuration needed to make Clef work with Bee.

-Ubuntu / Debian / Raspbian
-CentOS
-MacOS

## AMD64
-wget 'https://github.com/ethersphere/bee-clef/releases/download/v0.5.0/bee-clef_0.5.0_amd64.deb
-sudo-dpkg-i-bee-clef_0.5.0_amd64.deb`

## ARM (Raspberry Pi)

## ARMv7
-wget 'https://github.com/ethersphere/bee-clef/releases/download/v0.5.0/bee-clef_0.5.0_armv7.deb
-sudo-dpkg-i-bee-clef_0.5.0_armv7.deb`

## ARM64
-wget 'https://github.com/ethersphere/bee-clef/releases/download/v0.5.0/bee-clef_0.5.0_arm64.deb
-sudo-dpkg-i-bee-clef_0.5.0_arm64.deb`

## Finally, let's check Bee Clef is running.


- Linux
MacOS
systemctl 
status

 bee-clef
-bee-clef.service-Bee.Clef
     Loaded:
     loaded

 (/lib/systemd/system/bee-clef.service;
 enabled;
 vendor preset:
 enabled)
    -Active:
     active 
     (running)
     -since Fri 2020-11-20 23:45:16 GMT; 1min 29s ago

## Get in touch
[Only official website](https://www.ethswarm.org)


## License

This library is distributed under the BSD-style license found in the [LICENSE](LICENSE) file.

## Welcome!
Hello and welcome to the swarm! We are very happy to have you here with us! 🐝

As soon as your Bee client is up and running you will begin to connect with peers all over the world to become a part of Swarm, a global p2p network tasked with storing and distributing all of the world's data.

Swarm is a decentralised data storage and distribution technology, ready to power the next generation of censorship resistant, unstoppable serverless apps.

Swarm is economically self-sustaining due to a built-in incentive system enforced through smart contracts on the Ethereum blockchain. Swarm aspires to shape the future towards a self-sovereign global society and permissionless open markets. Applications can run autonomously yet securely in a planetary-scale deployment and execution environment.

## Bee
Installation#
Don't have Bee installed yet? It's easy! Head over to the installation section to get Bee up and running.

## Working With Bee
Once you have Bee installed, find out how to configure your software, interact with the API, monitor what Bee is up to, and make those all important backups in the working with Bee section.

## Configure Bee
Because Bee has many use cases and may run on a wide range of hardware, it is important that you configure Bee for your specific use case. This will make sure that you get the most out of your Bee!

-Important Configuration Parameters#
Bee is a versatile piece of software with diverse use cases. Before starting Bee for the first time, please consider changing the following configuration parameters to suit your needs. Read on for more specific information on how to tune your Bee, and (re)start it's service.

-Mainnet Node or Testnet Node#
To connect to mainnet, set your mainnet flag to true and network-id flag to 1.

mainnet: true
network-id: 1
Full Node or Light Node#
Since Bee can take a lot of resources when providing services to the network in exchange for BZZ, Bee nodes default automatically to running as a light node. To allow your Bee to use your network bandwidth and computing resources to serve the network and start cashing out cheques, set the --full-node flag to true.

full-node: true
Blockchain Endpoints#
Your Bee node must have stable access to the XDAI blockchain, so that it can interact with and deploy your chequebook contract. You can run your own XDAI node or, use a provider instead - we recommend Getblock.

-By default, Bee expects a local XDAI node at ws://localhost:8545. To use an Ethereum RPC provider instead, change your configuration as follows:

-swap-endpoint: https://stake.getblock.io/mainnet/?api_key=your-api-key
If you would like to use your node to resolve ENS domain names, you must also provide the endpoint for an Ethereum mainnet RPC provider.

-resolver-options: ["https://mainnet.infura.io/v3/<<your-api-key>>"]
Open File Descriptors#
Bee is designed to work on a lot of different hardware configurations. To facilitate the exploration of this, during our beeta phase, we have given node operators access to leveldb's --db-open-files-limit. This helps determine the speed with which Bee can read and write to its database, and therefore its efficiency in forwarding and serving chunks. Some say setting this to much more than the default 200 leads to a much enhanced ability to participate in the swarm and get those BZZ! Share your experience in the #node-operators channel of our Discord server to help us make this process more automated in the future.

-db-open-files-limit: 2000
NAT Address#
Swarm is all about sharing and storing chunks of data. To enable other Bees (also known as peers) to connect to your Bee, you must broadcast your public IP address, and ensure that Bee is reachable on the correct p2p port (default 1634). We recommend that you manually configure your external IP and check connectivity to ensure your Bee is able to receive connections from other peers.

## First determine your public IP address:

-curl icanhazip.com
123.123.123.123
Then configure your node, including your p2p port (default 1634).

nat-addr: "123.123.123.123:1634"

## Access the Swarm
To learn more about how to get the most out of Bee, find out how to access the swarm section so you can share files with your friends, use Bee to host a website on a public Swarm Gateway, and much more!

## Incentives
Need even more incentive to get involved with the wonderful world of Swarm? Find out how you'll soon be earning BZZ tokens for storing and distributing your share of the world's data - sharing is caring!

## Find Out More
What happens with your Bee node when you start it up? Want to know more about the amazing Swarm technology behind Bee? Want to make your own client? Read The Book of Swarm, our 250 page epic guide to the future tech underpinning the Swarm network.

## Development
We'd love for you to join our efforts! Are you up to the challenge of helping us to create Bee and the other incredible technologies we're building on top of it? You are invited to contribute code to the Bee client or any of the other projects in Swarm's Ethersphere.

## Community
There is a vibrant and buzzing community behind Swarm - get involved in one of our group channels:

- Swarm
- Discord
- Twitter @ethswarm
- reddit channel
- Medium

## Reporting a bug
If your Bee isn't working, get in touch with the #bee-support channel on Discord or let us know on GitHub! 🐝

Thanks for beeing here, we wish you Love and Bees from the Swarm Team x

## Quick Start
Bee is a versatile piece of software that caters for a diverse array of use cases.

- Access the Network
If you want to interact with the Bee ecosystem in a decentralised way, but not earn BZZ by storing or forwarding chunks, simply run a Bee light node in the background on your laptop or desktop computer. This will enable direct access to the swarm from your web browser and other applications.

-Install Bee

Support the Network and Earn BZZ by Running a Full Node#
Earn BZZ and help keep the swarm strong by running your own full node. It's easy to set up your own Bee on a Raspberry Pi, cloud host, or any home computer that's connected to the internet.

-Install Bee

## Run Your Own Hive of Nodes
Take it to the next level by keeping a whole hive of Bees! We provide tooling and monitoring to help you manage large deployments of multiple Bee nodes: Bee Hives.
