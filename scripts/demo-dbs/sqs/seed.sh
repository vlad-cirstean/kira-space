#!/usr/bin/env bash
# Kira Studio — SQS demo seed. Runs *inside* the kira-sqs (LocalStack) container (docker exec -i
# ... < this file), using the `awslocal` CLI the localstack image ships on PATH. Seeds
# queues and messages through the CLI, so no AWS SDK dependency.
set -euo pipefail

create_queue() {
  awslocal sqs create-queue --queue-name "$1" >/dev/null
}

send_messages() {
  local queue_url count
  queue_url=$(awslocal sqs get-queue-url --queue-name "$1" --query QueueUrl --output text)
  count="$2"
  for i in $(seq 0 $((count - 1))); do
    awslocal sqs send-message --queue-url "$queue_url" --message-body "{\"seq\":${i}}" >/dev/null
  done
}

create_queue orders-queue
send_messages orders-queue 15

create_queue drain-queue
send_messages drain-queue 15

create_queue empty-queue
