#!/usr/bin/env bash
# Makes a CA and a serving certificate for the solas-pivot webhook, stores
# them in the Secret solas-pivot-tls, and sets the caBundle of the
# MutatingWebhookConfiguration solas-pivot. kubectl must point at the
# target cluster.
set -euo pipefail

if ! command -v openssl >/dev/null; then
  echo "gen-webhook-certs: openssl not found. Run through devbox, for example devbox run pivot-smoke." >&2
  exit 1
fi

ns=solas-system
svc=solas-pivot
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT

openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
  -keyout "$dir/ca.key" -out "$dir/ca.crt" -subj "/CN=solas-pivot-ca" 2>/dev/null
openssl req -newkey rsa:2048 -nodes \
  -keyout "$dir/tls.key" -out "$dir/tls.csr" -subj "/CN=$svc.$ns.svc" 2>/dev/null
printf 'subjectAltName=DNS:%s,DNS:%s.%s,DNS:%s.%s.svc\n' "$svc" "$svc" "$ns" "$svc" "$ns" >"$dir/ext.cnf"
openssl x509 -req -in "$dir/tls.csr" -CA "$dir/ca.crt" -CAkey "$dir/ca.key" \
  -CAcreateserial -days 365 -extfile "$dir/ext.cnf" -out "$dir/tls.crt" 2>/dev/null

kubectl -n "$ns" create secret tls solas-pivot-tls \
  --cert="$dir/tls.crt" --key="$dir/tls.key" --dry-run=client -o yaml | kubectl apply -f -
bundle=$(base64 <"$dir/ca.crt" | tr -d '\n')
kubectl patch mutatingwebhookconfiguration solas-pivot --type json \
  -p "[{\"op\":\"add\",\"path\":\"/webhooks/0/clientConfig/caBundle\",\"value\":\"$bundle\"}]"
