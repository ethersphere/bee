#!/bin/bash

cluster_name=$1
if [[ -z $cluster_name ]]; then
    echo "Cluster name has to be specified!"
    exit 1
fi

nodes=$(beekeeper print nodes --cluster-name $cluster_name  --log-verbosity 0)

zip_files="debug/pprof/goroutine?debug=0"
debug_text="debug/pprof/goroutine?debug=1 debug/pprof/goroutine?debug=2 metrics"
debug_json="health node addresses chainstate transactions"
business_json="peers reservestate blocklist topology balances consumed timesettlements settlements chequebook/cheque chequebook/balance wallet stamps batches"
api_json="tags pins"

for n in $nodes
do
  mkdir -p dump/"$n"/{zip_files,debug_text,debug_json,business_json,api_json}
  for e in $zip_files
  do
    curl -s -o dump/"$n"/zip_files/${e//[\/\"\:\<\>\|\?\*]/_}.gzip "$n"-debug.localhost/$e
  done
  for e in $debug_text
  do
    curl -s -o dump/"$n"/debug_text/${e//[\/\"\:\<\>\|\?\*]/_} "$n"-debug.localhost/$e
  done
  for e in $debug_json
  do
    curl -s -o dump/"$n"/debug_json/${e//[\/\"\:\<\>\|\?\*]/_}.json "$n"-debug.localhost/$e
  done
  for e in $business_json
  do
    curl -s -o dump/"$n"/business_json/${e//[\/\"\:\<\>\|\?\*]/_}.json "$n"-debug.localhost/$e
  done
  for e in $api_json
  do
    curl -s -o dump/"$n"/api_json/${e//[\/\"\:\<\>\|\?\*]/_}.json "$n".localhost/$e
  done
done
kubectl -n local get pods > dump/kubectl_get_pods
kubectl -n local logs -l app.kubernetes.io/part-of=bee --tail -1 --prefix -c bee > dump/kubectl_logs
endpoint=$AWS_ENDPOINT
if [[ "$endpoint" != http* ]]
then
  endpoint=https://$endpoint
fi
fname=artifacts_${GITHUB_RUN_ID}.tar.gz
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
