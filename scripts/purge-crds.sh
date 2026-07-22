#!/usr/bin/env bash
set -euo pipefail

resources=(
  sandboxtemplates.sandbox.kce.ksyun.com
  sandboxes.sandbox.kce.ksyun.com
  sandboxclaims.sandbox.kce.ksyun.com
)

require() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "$1 is required" >&2
    exit 1
  fi
}

require kubectl

has_resources=false
for resource in "${resources[@]}"; do
  if ! kubectl get crd "${resource}" >/dev/null 2>&1; then
    continue
  fi

  instances="$(kubectl get "${resource}" --all-namespaces -o name)"
  if [[ -n "${instances}" ]]; then
    echo "cannot delete ${resource}: custom resources still exist:" >&2
    echo "${instances}" >&2
    has_resources=true
  fi
done

if [[ "${has_resources}" == "true" ]]; then
  cat >&2 <<'EOF'
Delete the listed CRs and wait for their finalizers to complete while the
operator is still running, then run this command again.
EOF
  exit 1
fi

kubectl delete crd "${resources[@]}" --ignore-not-found --wait=true
