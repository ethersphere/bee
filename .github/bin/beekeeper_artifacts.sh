#!/bin/bash

nodes="bootnode-0 bee-0 bee-1 light-0 light-1"

for i in $nodes
do
  mkdir -p dump/"$i"
  curl -s -o dump/"$i"/addresses.json "$i"-debug.localhost/addresses
  curl -s -o dump/"$i"/metrics "$i"-debug.localhost/metrics
  curl -s -o dump/"$i"/topology.json "$i"-debug.localhost/topology
  curl -s -o dump/"$i"/settlements.json "$i"-debug.localhost/settlements
  curl -s -o dump/"$i"/balances.json "$i"-debug.localhost/balances
  curl -s -o dump/"$i"/timesettlements.json "$i"-debug.localhost/timesettlements
  curl -s -o dump/"$i"/stamps.json "$i"-debug.localhost/stamps
done
kubectl -n local get pods > dump/kubectl_get_pods
kubectl -n local logs bee-0 > dump/kubectl_logs_bee_0
kubectl -n local logs bee-1 > dump/kubectl_logs_bee_1
kubectl -n local logs bootnode-0 > dump/kubectl_logs_bootnode_0
endpoint=$AWS_ENDPOINT
if [[ "$endpoint" != http* ]]
then
  endpoint=https://$endpoint
fi
fname=artifacts_${VERTAG}.tar.gz
tar -cz dump | aws --endpoint-url "$endpoint" s3 cp - s3://"$BUCKET_NAME"/"$fname"
aws --endpoint-url "$endpoint" s3api put-object-acl --bucket "$BUCKET_NAME" --acl public-read --key "$fname"
out="== Uploaded debugging artifacts to https://${BUCKET_NAME}.${AWS_ENDPOINT}/$fname =="
ln=${#out}
while [ "$ln" -gt 0 ]; do printf '=%.0s' '='; ((ln--));done;
echo ""
echo "$out"
ln=${#out}
while [ "$ln" -gt 0 ]; do printf '=%.0s' '='; ((ln--));done;
echo ""
