#!/usr/bin/env bash
set -euo pipefail

IMAGE="${1:-${IMAGE:-sandbox-operator:latest}}"

docker build -t "${IMAGE}" .

echo "Built ${IMAGE}"
