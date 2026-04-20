#!/bin/sh
# Admin-credentials verifier. Reads ROOT_ACCESS_KEY/ROOT_SECRET_KEY from
# environment (sourced via envFrom from versitygw-root-credentials Secret).
# Optionally S3_ENDPOINT / ADMIN_ENDPOINT overrides.
#
# Usage: admin.sh <op> [args...]
#
# Ops:
#   head-bucket-by-name <bucket>
#   head-bucket-not-found <bucket>             # must fail with NoSuchBucket
#   head-object <bucket> <key>
#   put-object <bucket> <key> <data>
#   delete-bucket-by-name <bucket>
#   list-users                                  # raw output of PATCH /list-users
#   assert-user-absent <expected-absent-access-key>
set -eu

: "${ROOT_ACCESS_KEY:?ROOT_ACCESS_KEY must be set via envFrom (Secret key: rootAccessKeyId)}"
: "${ROOT_SECRET_KEY:?ROOT_SECRET_KEY must be set via envFrom (Secret key: rootSecretAccessKey)}"

export AWS_ACCESS_KEY_ID="$ROOT_ACCESS_KEY"
export AWS_SECRET_ACCESS_KEY="$ROOT_SECRET_KEY"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"

S3="${S3_ENDPOINT:-http://versitygw.versitygw-system.svc.cluster.local:7070}"
ADMIN="${ADMIN_ENDPOINT:-http://versitygw.versitygw-system.svc.cluster.local:7071}"

OP="${1:-}"
shift || true

case "$OP" in
  head-bucket-by-name)
    BUCKET="${1:?usage: head-bucket-by-name <bucket>}"
    aws s3api head-bucket --endpoint-url "$S3" --bucket "$BUCKET"
    ;;
  head-bucket-not-found)
    BUCKET="${1:?usage: head-bucket-not-found <bucket>}"
    set +e
    ERR=$(aws s3api head-bucket --endpoint-url "$S3" --bucket "$BUCKET" 2>&1)
    RC=$?
    set -e
    if [ "$RC" -eq 0 ]; then
      echo "expected NoSuchBucket but head-bucket succeeded for '$BUCKET'" >&2
      exit 4
    fi
    echo "$ERR" | grep -qE 'NoSuchBucket|404|Not Found' || {
      echo "unexpected error: $ERR" >&2 ; exit 5
    }
    ;;
  head-object)
    BUCKET="${1:?usage: head-object <bucket> <key>}"
    KEY="${2:?usage: head-object <bucket> <key>}"
    aws s3api head-object --endpoint-url "$S3" --bucket "$BUCKET" --key "$KEY"
    ;;
  put-object)
    BUCKET="${1:?usage: put-object <bucket> <key> <data>}"
    KEY="${2:?usage: put-object <bucket> <key> <data>}"
    DATA="${3:?usage: put-object <bucket> <key> <data>}"
    echo -n "$DATA" > /tmp/body
    aws s3api put-object --endpoint-url "$S3" --bucket "$BUCKET" --key "$KEY" --body /tmp/body
    ;;
  delete-bucket-by-name)
    # Idempotent: tc-e-009 / tc-e-062 call this to clean up a bucket that
    # may or may not still exist, depending on whether the natural COSI
    # delete path or the finalizer-strip fallback won the race under
    # parallel load. NoSuchBucket is a success condition for cleanup.
    BUCKET="${1:?usage: delete-bucket-by-name <bucket>}"
    set +e
    ERR=$(aws s3api delete-bucket --endpoint-url "$S3" --bucket "$BUCKET" 2>&1)
    RC=$?
    set -e
    if [ "$RC" -eq 0 ]; then
      exit 0
    fi
    # Anchor to the AWS SDK's canonical NoSuchBucket marker; do NOT match
    # bare "404" or "Not Found" (they'd swallow unrelated failures like
    # proxy 404s or throttle responses).
    if echo "$ERR" | grep -qE 'NoSuchBucket|The specified bucket does not exist'; then
      exit 0
    fi
    echo "$ERR" >&2
    exit "$RC"
    ;;
  list-users)
    # VersityGW admin API is SigV4-signed PATCH on $ADMIN.
    curl -sS -f -X PATCH "$ADMIN/list-users" \
      --aws-sigv4 "aws:amz:$AWS_DEFAULT_REGION:s3" \
      --user "$AWS_ACCESS_KEY_ID:$AWS_SECRET_ACCESS_KEY"
    ;;
  assert-user-absent)
    KEY="${1:?usage: assert-user-absent <access-key-id>}"
    OUT=$(curl -sS -f -X PATCH "$ADMIN/list-users" \
      --aws-sigv4 "aws:amz:$AWS_DEFAULT_REGION:s3" \
      --user "$AWS_ACCESS_KEY_ID:$AWS_SECRET_ACCESS_KEY")
    if echo "$OUT" | grep -q "$KEY"; then
      echo "user '$KEY' is still present in list-users output:" >&2
      echo "$OUT" >&2
      exit 6
    fi
    ;;
  *)
    echo "unknown admin-mode op: '$OP'" >&2
    echo "see $0 header for usage" >&2
    exit 64
    ;;
esac
