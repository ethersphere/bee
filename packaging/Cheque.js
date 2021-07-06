curl localhost:1635/chequebook/cheque | jq
{
  "totalreceived": 0,
  "totalsent": 718030,
  "settlements": [
    //...
    {
      "peer": "dce1833609db868e7611145b48224c061ea57fd14e784a278f2469f355292ca6",
      "received": 0,
      "sent": 89550
    }
    //...
  ]
}
