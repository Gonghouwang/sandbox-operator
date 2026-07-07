#!/usr/bin/env bash
set -euo pipefail

kubectl delete -f config/deploy/05-webhook.yaml --ignore-not-found
kubectl delete -f config/deploy/04-manager.yaml --ignore-not-found
kubectl delete -f config/deploy/03-config.yaml --ignore-not-found
kubectl delete -f config/deploy/02-rbac.yaml --ignore-not-found
kubectl delete -f config/deploy/01-crd.yaml --ignore-not-found
kubectl delete -f config/deploy/00-namespace.yaml --ignore-not-found
