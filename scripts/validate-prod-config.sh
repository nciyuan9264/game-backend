#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

config="$(docker compose -f docker-compose.prod.yml config)"

reject() {
  local pattern="$1"
  local message="$2"
  if grep -Eiq -- "$pattern" <<<"$config"; then
    printf 'production config rejected: %s\n' "$message" >&2
    exit 1
  fi
}

require() {
  local pattern="$1"
  local message="$2"
  if ! grep -Eiq -- "$pattern" <<<"$config"; then
    printf 'production config rejected: %s\n' "$message" >&2
    exit 1
  fi
}

reject '(CHANGE_ME|your-instance|example\.com)' 'placeholder values in .env must be replaced'
reject 'neon\.tech' 'legacy Neon endpoints are forbidden'
reject 'sslmode=disable' 'PostgreSQL TLS must not be disabled'
reject 'POSTGRES_DSN:.*@(postgres|localhost|127\.0\.0\.1)(:|/)' 'PostgreSQL must use the remote RDS endpoint'
require 'POSTGRES_DSN:.*\.rds\.aliyuncs\.com(:5432)?/gamebus_prod([?[:space:]]|$)' 'PostgreSQL must use the shared Alibaba Cloud RDS gamebus_prod database'
require 'POSTGRES_DSN:.*[?&]sslmode=require' 'PostgreSQL must require TLS'

printf 'production compose configuration passed validation\n'
