// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postagecontract

const PostageABI = `[
{
"inputs": [
{
"internalType": "address",
"name": "_bzzToken",
"type": "address"
}
],
"stateMutability": "nonpayable",
"type": "constructor"
},
{
"anonymous": false,
"inputs": [
{
"indexed": true,
"internalType": "bytes32",
"name": "batchId",
"type": "bytes32"
},
{
"indexed": false,
"internalType": "uint256",
"name": "totalAmount",
"type": "uint256"
},
{
"indexed": false,
"internalType": "uint256",
"name": "normalisedBalance",
"type": "uint256"
},
{
"indexed": false,
"internalType": "address",
"name": "owner",
"type": "address"
},
{
"indexed": false,
"internalType": "uint8",
"name": "depth",
"type": "uint8"
},
{
"indexed": false,
"internalType": "uint8",
"name": "bucketDepth",
"type": "uint8"
},
{
"indexed": false,
"internalType": "bool",
"name": "immutableFlag",
"type": "bool"
}
],
"name": "BatchCreated",
"type": "event"
},
{
"anonymous": false,
"inputs": [
{
"indexed": true,
"internalType": "bytes32",
"name": "batchId",
"type": "bytes32"
},
{
"indexed": false,
"internalType": "uint8",
"name": "newDepth",
"type": "uint8"
},
{
"indexed": false,
"internalType": "uint256",
"name": "normalisedBalance",
"type": "uint256"
}
],
"name": "BatchDepthIncrease",
"type": "event"
},
{
"anonymous": false,
"inputs": [
{
"indexed": true,
"internalType": "bytes32",
"name": "batchId",
"type": "bytes32"
},
{
"indexed": false,
"internalType": "uint256",
"name": "topupAmount",
"type": "uint256"
},
{
"indexed": false,
"internalType": "uint256",
"name": "normalisedBalance",
"type": "uint256"
}
],
"name": "BatchTopUp",
"type": "event"
},
{
"anonymous": false,
"inputs": [
{
"indexed": false,
"internalType": "address",
"name": "account",
"type": "address"
}
],
"name": "Paused",
"type": "event"
},
{
"anonymous": false,
"inputs": [
{
"indexed": false,
"internalType": "uint256",
"name": "price",
"type": "uint256"
}
],
"name": "PriceUpdate",
"type": "event"
},
{
"anonymous": false,
"inputs": [
{
"indexed": true,
"internalType": "bytes32",
"name": "role",
"type": "bytes32"
},
{
"indexed": true,
"internalType": "bytes32",
"name": "previousAdminRole",
"type": "bytes32"
},
{
"indexed": true,
"internalType": "bytes32",
"name": "newAdminRole",
"type": "bytes32"
}
],
"name": "RoleAdminChanged",
"type": "event"
},
{
"anonymous": false,
"inputs": [
{
"indexed": true,
"internalType": "bytes32",
"name": "role",
"type": "bytes32"
},
{
"indexed": true,
"internalType": "address",
"name": "account",
"type": "address"
},
{
"indexed": true,
"internalType": "address",
"name": "sender",
"type": "address"
}
],
"name": "RoleGranted",
"type": "event"
},
{
"anonymous": false,
"inputs": [
{
"indexed": true,
"internalType": "bytes32",
"name": "role",
"type": "bytes32"
},
{
"indexed": true,
"internalType": "address",
"name": "account",
"type": "address"
},
{
"indexed": true,
"internalType": "address",
"name": "sender",
"type": "address"
}
],
"name": "RoleRevoked",
"type": "event"
},
{
"anonymous": false,
"inputs": [
{
"indexed": false,
"internalType": "address",
"name": "account",
"type": "address"
}
],
"name": "Unpaused",
"type": "event"
},
{
"inputs": [],
"name": "DEFAULT_ADMIN_ROLE",
"outputs": [
{
"internalType": "bytes32",
"name": "",
"type": "bytes32"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [],
"name": "DEPTH_ORACLE_ROLE",
"outputs": [
{
"internalType": "bytes32",
"name": "",
"type": "bytes32"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [],
"name": "PAUSER_ROLE",
"outputs": [
{
"internalType": "bytes32",
"name": "",
"type": "bytes32"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [],
"name": "PRICE_ORACLE_ROLE",
"outputs": [
{
"internalType": "bytes32",
"name": "",
"type": "bytes32"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [],
"name": "REDISTRIBUTOR_ROLE",
"outputs": [
{
"internalType": "bytes32",
"name": "",
"type": "bytes32"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [
{
"internalType": "bytes32",
"name": "",
"type": "bytes32"
}
],
"name": "batches",
"outputs": [
{
"internalType": "address",
"name": "owner",
"type": "address"
},
{
"internalType": "uint8",
"name": "depth",
"type": "uint8"
},
{
"internalType": "bool",
"name": "immutableFlag",
"type": "bool"
},
{
"internalType": "uint256",
"name": "normalisedBalance",
"type": "uint256"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [],
"name": "bzzToken",
"outputs": [
{
"internalType": "address",
"name": "",
"type": "address"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [
{
"internalType": "address",
"name": "_owner",
"type": "address"
},
{
"internalType": "uint256",
"name": "_initialBalancePerChunk",
"type": "uint256"
},
{
"internalType": "uint8",
"name": "_depth",
"type": "uint8"
},
{
"internalType": "uint8",
"name": "_bucketDepth",
"type": "uint8"
},
{
"internalType": "bytes32",
"name": "_batchId",
"type": "bytes32"
},
{
"internalType": "bool",
"name": "_immutable",
"type": "bool"
}
],
"name": "copyBatch",
"outputs": [],
"stateMutability": "nonpayable",
"type": "function"
},
{
"inputs": [
{
"internalType": "address",
"name": "_owner",
"type": "address"
},
{
"internalType": "uint256",
"name": "_initialBalancePerChunk",
"type": "uint256"
},
{
"internalType": "uint8",
"name": "_depth",
"type": "uint8"
},
{
"internalType": "uint8",
"name": "_bucketDepth",
"type": "uint8"
},
{
"internalType": "bytes32",
"name": "_nonce",
"type": "bytes32"
},
{
"internalType": "bool",
"name": "_immutable",
"type": "bool"
}
],
"name": "createBatch",
"outputs": [],
"stateMutability": "nonpayable",
"type": "function"
},
{
"inputs": [],
"name": "currentTotalOutPayment",
"outputs": [
{
"internalType": "uint256",
"name": "",
"type": "uint256"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [],
"name": "empty",
"outputs": [
{
"internalType": "bool",
"name": "",
"type": "bool"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [
{
"internalType": "uint256",
"name": "limit",
"type": "uint256"
}
],
"name": "expireLimited",
"outputs": [],
"stateMutability": "nonpayable",
"type": "function"
},
{
"inputs": [],
"name": "expiredBatchesExist",
"outputs": [
{
"internalType": "bool",
"name": "",
"type": "bool"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [],
"name": "firstBatchId",
"outputs": [
{
"internalType": "bytes32",
"name": "",
"type": "bytes32"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [
{
"internalType": "bytes32",
"name": "role",
"type": "bytes32"
}
],
"name": "getRoleAdmin",
"outputs": [
{
"internalType": "bytes32",
"name": "",
"type": "bytes32"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [
{
"internalType": "bytes32",
"name": "role",
"type": "bytes32"
},
{
"internalType": "address",
"name": "account",
"type": "address"
}
],
"name": "grantRole",
"outputs": [],
"stateMutability": "nonpayable",
"type": "function"
},
{
"inputs": [
{
"internalType": "bytes32",
"name": "role",
"type": "bytes32"
},
{
"internalType": "address",
"name": "account",
"type": "address"
}
],
"name": "hasRole",
"outputs": [
{
"internalType": "bool",
"name": "",
"type": "bool"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [
{
"internalType": "bytes32",
"name": "_batchId",
"type": "bytes32"
},
{
"internalType": "uint8",
"name": "_newDepth",
"type": "uint8"
}
],
"name": "increaseDepth",
"outputs": [],
"stateMutability": "nonpayable",
"type": "function"
},
{
"inputs": [],
"name": "lastExpiryBalance",
"outputs": [
{
"internalType": "uint256",
"name": "",
"type": "uint256"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [],
"name": "lastPrice",
"outputs": [
{
"internalType": "uint256",
"name": "",
"type": "uint256"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [],
"name": "lastUpdatedBlock",
"outputs": [
{
"internalType": "uint256",
"name": "",
"type": "uint256"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [],
"name": "minimumBatchDepth",
"outputs": [
{
"internalType": "uint8",
"name": "",
"type": "uint8"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [],
"name": "pause",
"outputs": [],
"stateMutability": "nonpayable",
"type": "function"
},
{
"inputs": [],
"name": "paused",
"outputs": [
{
"internalType": "bool",
"name": "",
"type": "bool"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [],
"name": "pot",
"outputs": [
{
"internalType": "uint256",
"name": "",
"type": "uint256"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [
{
"internalType": "bytes32",
"name": "_batchId",
"type": "bytes32"
}
],
"name": "remainingBalance",
"outputs": [
{
"internalType": "uint256",
"name": "",
"type": "uint256"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [
{
"internalType": "bytes32",
"name": "role",
"type": "bytes32"
},
{
"internalType": "address",
"name": "account",
"type": "address"
}
],
"name": "renounceRole",
"outputs": [],
"stateMutability": "nonpayable",
"type": "function"
},
{
"inputs": [
{
"internalType": "bytes32",
"name": "role",
"type": "bytes32"
},
{
"internalType": "address",
"name": "account",
"type": "address"
}
],
"name": "revokeRole",
"outputs": [],
"stateMutability": "nonpayable",
"type": "function"
},
{
"inputs": [
{
"internalType": "uint8",
"name": "min",
"type": "uint8"
}
],
"name": "setMinimumBatchDepth",
"outputs": [],
"stateMutability": "nonpayable",
"type": "function"
},
{
"inputs": [
{
"internalType": "uint256",
"name": "_price",
"type": "uint256"
}
],
"name": "setPrice",
"outputs": [],
"stateMutability": "nonpayable",
"type": "function"
},
{
"inputs": [
{
"internalType": "bytes4",
"name": "interfaceId",
"type": "bytes4"
}
],
"name": "supportsInterface",
"outputs": [
{
"internalType": "bool",
"name": "",
"type": "bool"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [
{
"internalType": "bytes32",
"name": "_batchId",
"type": "bytes32"
},
{
"internalType": "uint256",
"name": "_topupAmountPerChunk",
"type": "uint256"
}
],
"name": "topUp",
"outputs": [],
"stateMutability": "nonpayable",
"type": "function"
},
{
"inputs": [
{
"internalType": "uint256",
"name": "amount",
"type": "uint256"
}
],
"name": "topupPot",
"outputs": [],
"stateMutability": "nonpayable",
"type": "function"
},
{
"inputs": [],
"name": "totalPot",
"outputs": [
{
"internalType": "uint256",
"name": "",
"type": "uint256"
}
],
"stateMutability": "nonpayable",
"type": "function"
},
{
"inputs": [],
"name": "unPause",
"outputs": [],
"stateMutability": "nonpayable",
"type": "function"
},
{
"inputs": [],
"name": "validChunkCount",
"outputs": [
{
"internalType": "uint256",
"name": "",
"type": "uint256"
}
],
"stateMutability": "view",
"type": "function"
},
{
"inputs": [
{
"internalType": "address",
"name": "beneficiary",
"type": "address"
}
],
"name": "withdraw",
"outputs": [],
"stateMutability": "nonpayable",
"type": "function"
}
]`
