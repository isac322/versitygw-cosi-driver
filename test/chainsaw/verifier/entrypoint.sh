#!/bin/sh
# User-credentials verifier. Reads BucketInfo from a mounted COSI Secret and
# performs the requested S3 operation. Exits 0 on success, non-zero on failure.
#
# Usage: entrypoint.sh [<op> [args...]]
#
# Ops:
#   (no op) | idle                          # long-running idle (for kubectl exec)
#   head-bucket
#   put-object <key> <data>
#   get-object <key> <expected-content>
#   list-objects <expected-key> [expected-key ...]
#   expect-403 <op> [args...]              # inner op must fail with HTTP 403
#   expect-nosuchbucket <op> [args...]     # inner op must fail with NoSuchBucket
#   extract-access-key                     # write accessKeyID to /output/access-key
set -eu

# Idle mode: start the container as a long-running shim so recovery tests can
# reuse it via `kubectl exec` instead of spawning a fresh Job per verification.
# BucketInfo is re-parsed on each exec invocation below, so it doesn't need to
# exist at container start time in this mode.
case "${1:-}" in
  ""|idle)
    exec sleep infinity
    ;;
esac

BI="${BUCKETINFO_PATH:-/conf/BucketInfo}"

if [ ! -f "$BI" ]; then
  echo "BucketInfo not found at $BI" >&2
  exit 64
fi

ACCESS=$(jq -r '.spec.secretS3.accessKeyID' "$BI")
SECRET=$(jq -r '.spec.secretS3.accessSecretKey' "$BI")
ENDPOINT=$(jq -r '.spec.secretS3.endpoint' "$BI")
REGION=$(jq -r '.spec.secretS3.region' "$BI")
BUCKET=$(jq -r '.spec.bucketName' "$BI")

if [ -z "$REGION" ] || [ "$REGION" = "null" ]; then
  REGION="us-east-1"
fi

export AWS_ACCESS_KEY_ID="$ACCESS"
export AWS_SECRET_ACCESS_KEY="$SECRET"
export AWS_DEFAULT_REGION="$REGION"

OP="${1:-}"
shift || true

expect_error_pattern() {
  PATTERN="$1"; shift
  # $@ is the inner op + its args.
  set +e
  ERR=$("$0" "$@" 2>&1 >/tmp/expect.out)
  RC=$?
  set -e
  if [ "$RC" -eq 0 ]; then
    echo "expected failure matching /$PATTERN/ but inner op succeeded" >&2
    cat /tmp/expect.out >&2 || true
    exit 4
  fi
  # Match against combined stderr (ERR captured) and stdout (/tmp/expect.out).
  if echo "$ERR" | grep -qE "$PATTERN"; then
    exit 0
  fi
  if cat /tmp/expect.out 2>/dev/null | grep -qE "$PATTERN"; then
    exit 0
  fi
  echo "expected failure matching /$PATTERN/ but got:" >&2
  echo "--stderr--" >&2 ; echo "$ERR" >&2
  echo "--stdout--" >&2 ; cat /tmp/expect.out >&2 || true
  exit 5
}

case "$OP" in
  head-bucket)
    aws s3api head-bucket --endpoint-url "$ENDPOINT" --bucket "$BUCKET"
    ;;
  put-object)
    KEY="${1:?usage: put-object <key> <data>}"
    DATA="${2:?usage: put-object <key> <data>}"
    echo -n "$DATA" > /tmp/body
    aws s3api put-object --endpoint-url "$ENDPOINT" --bucket "$BUCKET" --key "$KEY" --body /tmp/body
    ;;
  get-object)
    KEY="${1:?usage: get-object <key> <expected-content>}"
    EXPECTED="${2:?usage: get-object <key> <expected-content>}"
    aws s3api get-object --endpoint-url "$ENDPOINT" --bucket "$BUCKET" --key "$KEY" /tmp/got
    ACTUAL=$(cat /tmp/got)
    if [ "$ACTUAL" != "$EXPECTED" ]; then
      echo "content mismatch: got='$ACTUAL' want='$EXPECTED'" >&2
      exit 2
    fi
    ;;
  list-objects)
    OUT=$(aws s3api list-objects-v2 --endpoint-url "$ENDPOINT" --bucket "$BUCKET" --query 'Contents[].Key' --output text)
    for key in "$@"; do
      echo "$OUT" | tr '\t' '\n' | grep -qx "$key" || {
        echo "missing key: $key; got: $OUT" >&2
        exit 3
      }
    done
    ;;
  expect-403)
    # VersityGW may return 403 AccessDenied/Forbidden, but also
    # SignatureDoesNotMatch, InvalidAccessKeyId, or XAdminNoSuchUser after
    # account deletion. Accept any authentication/authorization failure.
    expect_error_pattern 'AccessDenied|403|Forbidden|SignatureDoesNotMatch|InvalidAccessKeyId|XAdminNoSuchUser|InvalidAccessKeyID|NotAuthorized|401' "$@"
    ;;
  expect-nosuchbucket)
    expect_error_pattern 'NoSuchBucket|404|Not Found' "$@"
    ;;
  extract-access-key)
    mkdir -p /output
    printf '%s' "$ACCESS" > /output/access-key
    ;;
  *)
    echo "unknown user-mode op: '$OP'" >&2
    echo "see $0 header for usage" >&2
    exit 64
    ;;
esac
