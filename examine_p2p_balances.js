It is also possible to examine your per-peer balances.

curl localhost:1635/balances | jq
{
  "balances": [
    //...
    {
      "peer": "d0bf001e05014fa036af97f3d226bee253d2b147f540b6c2210947e5b7b409af",
      "balance": -85420
    },
    {
      "peer": "f1e2872581de18bdc68060dc8edd3aa96368eb341e915aba86b450486b105a47",
      "balance": -75990
    }
    //...
  ]
