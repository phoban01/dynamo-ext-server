#!/usr/bin/env bash
# Makes a CA and a serving certificate for solas-apiserver, stores them in
# the Secret solas-apiserver-tls, and sets the caBundle of the APIService.
# It needs kubectl to point at the target cluster.
set -euo pipefail

ns=solas-system
svc=solas-apiserver
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT

openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
  -keyout "$dir/ca.key" -out "$dir/ca.crt" -subj "/CN=solas-ca" 2>/dev/null
openssl req -newkey rsa:2048 -nodes \
  -keyout "$dir/tls.key" -out "$dir/tls.csr" -subj "/CN=$svc.$ns.svc" 2>/dev/null
printf 'subjectAltName=DNS:%s,DNS:%s.%s,DNS:%s.%s.svc\n' "$svc" "$svc" "$ns" "$svc" "$ns" >"$dir/ext.cnf"
openssl x509 -req -in "$dir/tls.csr" -CA "$dir/ca.crt" -CAkey "$dir/ca.key" \
  -CAcreateserial -days 365 -extfile "$dir/ext.cnf" -out "$dir/tls.crt" 2>/dev/null

kubectl -n "$ns" create secret tls solas-apiserver-tls \
  --cert="$dir/tls.crt" --key="$dir/tls.key" --dry-run=client -o yaml | kubectl apply -f -
bundle=$(base64 <"$dir/ca.crt" | tr -d '\n')
kubectl patch apiservice v1alpha1.solas.dev --type merge -p "{\"spec\":{\"caBundle\":\"$bundle\"}}"
