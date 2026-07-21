#!/usr/bin/env bash
set -euo pipefail

IMAGE="${IMAGE:-hub.kce.ksyun.com/ksyun-public/sandbox-operator:v20260707}"
IMAGE_PULL_SECRET="${IMAGE_PULL_SECRET:-}"
NAMESPACE="${NAMESPACE:-sandbox-operator-system}"
SERVICE_NAME="${SERVICE_NAME:-sandbox-operator-webhook}"
TLS_SECRET="${TLS_SECRET:-sandbox-operator-webhook-server-cert}"
OPENAPI_BASE_URL="${OPENAPI_BASE_URL:-http://aicp.cn-beijing-6.api.ksyun.com}"

require() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "$1 is required" >&2
    exit 1
  fi
}

validate_namespace() {
  if [[ ! "${NAMESPACE}" =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ ]]; then
    echo "NAMESPACE must be a valid Kubernetes namespace name: ${NAMESPACE}" >&2
    exit 1
  fi
}

render_manifest() {
  local escaped_openapi_base_url
  escaped_openapi_base_url="$(printf '%s' "${OPENAPI_BASE_URL}" | sed 's/[\\&|]/\\\\&/g')"
  sed \
    -e "s|sandbox-operator-system|${NAMESPACE}|g" \
    -e "s|http://aicp.cn-beijing-6.api.ksyun.com|${escaped_openapi_base_url}|g" \
    "$1"
}

apply_manifest() {
  render_manifest "$1" | kubectl apply -f -
}

generate_webhook_cert() {
  require openssl
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "${tmpdir}"' EXIT

  cat >"${tmpdir}/csr.conf" <<EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[v3_req]
basicConstraints = CA:FALSE
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names
[alt_names]
DNS.1 = ${SERVICE_NAME}
DNS.2 = ${SERVICE_NAME}.${NAMESPACE}
DNS.3 = ${SERVICE_NAME}.${NAMESPACE}.svc
DNS.4 = ${SERVICE_NAME}.${NAMESPACE}.svc.cluster.local
EOF

  openssl genrsa -out "${tmpdir}/ca.key" 2048 >/dev/null 2>&1
  openssl req -x509 -new -nodes -key "${tmpdir}/ca.key" -sha256 -days 3650 \
    -subj "/CN=${SERVICE_NAME}.${NAMESPACE}.svc-ca" \
    -out "${tmpdir}/ca.crt" >/dev/null 2>&1
  openssl genrsa -out "${tmpdir}/tls.key" 2048 >/dev/null 2>&1
  openssl req -new -key "${tmpdir}/tls.key" \
    -subj "/CN=${SERVICE_NAME}.${NAMESPACE}.svc" \
    -out "${tmpdir}/tls.csr" \
    -config "${tmpdir}/csr.conf" >/dev/null 2>&1
  openssl x509 -req -in "${tmpdir}/tls.csr" \
    -CA "${tmpdir}/ca.crt" -CAkey "${tmpdir}/ca.key" -CAcreateserial \
    -out "${tmpdir}/tls.crt" -days 3650 -sha256 \
    -extensions v3_req -extfile "${tmpdir}/csr.conf" >/dev/null 2>&1

  kubectl -n "${NAMESPACE}" create secret tls "${TLS_SECRET}" \
    --cert="${tmpdir}/tls.crt" \
    --key="${tmpdir}/tls.key" \
    --dry-run=client -o yaml | kubectl apply -f -

  CA_BUNDLE="$(base64 <"${tmpdir}/ca.crt" | tr -d '\n')"
}

patch_webhook_ca_bundle() {
  kubectl patch mutatingwebhookconfiguration sandbox-operator-mutating-webhook --type=json -p "[
    {\"op\":\"add\",\"path\":\"/webhooks/0/clientConfig/caBundle\",\"value\":\"${CA_BUNDLE}\"},
    {\"op\":\"add\",\"path\":\"/webhooks/1/clientConfig/caBundle\",\"value\":\"${CA_BUNDLE}\"},
    {\"op\":\"add\",\"path\":\"/webhooks/2/clientConfig/caBundle\",\"value\":\"${CA_BUNDLE}\"}
  ]"
  kubectl patch validatingwebhookconfiguration sandbox-operator-validating-webhook --type=json -p "[
    {\"op\":\"add\",\"path\":\"/webhooks/0/clientConfig/caBundle\",\"value\":\"${CA_BUNDLE}\"},
    {\"op\":\"add\",\"path\":\"/webhooks/1/clientConfig/caBundle\",\"value\":\"${CA_BUNDLE}\"},
    {\"op\":\"add\",\"path\":\"/webhooks/2/clientConfig/caBundle\",\"value\":\"${CA_BUNDLE}\"}
  ]"
}

validate_namespace
apply_manifest config/deploy/00-namespace.yaml
apply_manifest config/deploy/01-crd.yaml
apply_manifest config/deploy/02-rbac.yaml
apply_manifest config/deploy/03-config.yaml
generate_webhook_cert
apply_manifest config/deploy/04-manager.yaml
kubectl -n "${NAMESPACE}" set image deployment/sandbox-operator manager="${IMAGE}"
if [[ -n "${IMAGE_PULL_SECRET}" ]]; then
  kubectl -n "${NAMESPACE}" patch deployment sandbox-operator --type=merge \
    -p "{\"spec\":{\"template\":{\"spec\":{\"imagePullSecrets\":[{\"name\":\"${IMAGE_PULL_SECRET}\"}]}}}}"
fi
apply_manifest config/deploy/05-webhook.yaml
patch_webhook_ca_bundle
# ConfigMap values are consumed as container environment variables. Restart so a
# repeated deployment applies an overridden OpenAPI endpoint immediately.
kubectl -n "${NAMESPACE}" rollout restart deployment/sandbox-operator
kubectl -n "${NAMESPACE}" rollout status deployment/sandbox-operator
