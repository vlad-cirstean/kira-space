#!/usr/bin/env bash
# Kira Studio — S3 demo seed. Runs *inside* the kira-sqs (LocalStack) container (docker exec -i
# ... < this file), using the `awslocal` CLI the localstack image ships on PATH — the same one
# container sqs/seed.sh uses, since docker-compose.yml's SERVICES lists both. Seeds a
# bucket/prefix/object layout through the CLI, so no AWS SDK dependency.
#
# P33: grown from 3 objects in 2 buckets to a full size/type ladder plus a >1000-object prefix —
# every state the phase introduces (bounded preview, bounded edit, binary refusal, the
# ListObjectsV2 continuation loop) needs to be reachable by hand from this seed, not just from
# packages/db-fixtures/. `mb` calls are idempotent (`|| true`); re-running overwrites objects in place.
set -euo pipefail

awslocal s3 mb s3://kira-demo-bucket >/dev/null || true
awslocal s3 mb s3://kira-empty-bucket >/dev/null || true
# P33: the upload target — also the D17 case of a bucket with nothing in it to open an object
# from, so Upload has to be reachable from the tree's bucket row itself.
awslocal s3 mb s3://kira-uploads-bucket >/dev/null || true

echo -n "hello from the bucket root" |
  awslocal s3 cp - s3://kira-demo-bucket/readme.txt --content-type text/plain \
    --metadata seeded=true >/dev/null
echo -n '{"year":2024,"total":42}' |
  awslocal s3 cp - s3://kira-demo-bucket/reports/2024/summary.json --content-type application/json >/dev/null
echo -n "a sibling of the 2024/ prefix, under reports/ itself" |
  awslocal s3 cp - s3://kira-demo-bucket/reports/notes.txt --content-type text/plain >/dev/null
# A key with spaces and parentheses — exercises path encoding through download, delete and the
# tab title, not just the happy-path ASCII keys above.
echo -n '{"quarter":"Q1"}' |
  awslocal s3 cp - "s3://kira-demo-bucket/reports/quarter one (Q1).json" \
    --content-type application/json >/dev/null

# The size/type ladder (P33 D4/D6/D7's own thresholds: OBJECT_BODY_EDIT_BYTES = 1 MB,
# SINGLE_ROW_MAX_BYTES = 4 MB).
echo -n "" |
  awslocal s3 cp - s3://kira-demo-bucket/sizes/tiny.txt --content-type text/plain >/dev/null
head -c 4000 /dev/zero | tr '\0' 'a' |
  awslocal s3 cp - s3://kira-demo-bucket/sizes/small.json --content-type application/json >/dev/null
head -c 512000 /dev/zero | tr '\0' 'b' |
  awslocal s3 cp - s3://kira-demo-bucket/sizes/medium.csv --content-type text/csv >/dev/null
# Between the two thresholds: renders, but Edit is disabled with the actual size named.
head -c 2000000 /dev/zero | tr '\0' 'c' |
  awslocal s3 cp - s3://kira-demo-bucket/sizes/large.log --content-type text/plain >/dev/null
# Over the preview threshold: no Body row at all, Download is the only way to see it.
head -c 8000000 /dev/zero | tr '\0' 'd' |
  awslocal s3 cp - s3://kira-demo-bucket/sizes/huge.bin --content-type application/octet-stream >/dev/null
# A real, tiny PNG (1x1, transparent) — small enough to preview, but its bytes are not valid
# UTF-8, so Edit is refused with the not-UTF-8 reason (D7) while the lossy preview still renders.
echo -n 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=' |
  base64 -d |
  awslocal s3 cp - s3://kira-demo-bucket/sizes/logo.png --content-type image/png >/dev/null

# Past ListObjectsV2's 1000-key-per-page default — exercises listPrefixChildren's continuation
# loop, which the 3-object seed never touched. Written to a local directory first and synced in
# one call rather than 1,200 individual `s3 cp` invocations.
bulk_dir="$(mktemp -d)"
for i in $(seq -w 1 1200); do
  echo -n "{\"item\":${i}}" > "${bulk_dir}/item-${i}.json"
done
awslocal s3 sync --quiet --content-type application/json "${bulk_dir}" s3://kira-demo-bucket/bulk/ >/dev/null
rm -rf "${bulk_dir}"
