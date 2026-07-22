#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-sandbox-operator-system}"

validate_namespace() {
  if [[ ! "${NAMESPACE}" =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ ]]; then
    echo "NAMESPACE must be a valid Kubernetes namespace name: ${NAMESPACE}" >&2
    exit 1
  fi
}

delete_manifest() {
  sed "s|sandbox-operator-system|${NAMESPACE}|g" "$1" | kubectl delete -f - --ignore-not-found
}

validate_namespace
delete_manifest config/deploy/05-webhook.yaml
delete_manifest config/deploy/04-manager.yaml
delete_manifest config/deploy/03-config.yaml
delete_manifest config/deploy/02-rbac.yaml
# Preserve CRDs and their business resources. Deleting a CRD while CRs still
# carry operator finalizers can block CRD cleanup after the controller is gone.
delete_manifest config/deploy/00-namespace.yaml
