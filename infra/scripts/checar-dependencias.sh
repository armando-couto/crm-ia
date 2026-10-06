#!/usr/bin/env bash
# Confere as ferramentas necessárias para subir a plataforma.
set -u
ok=1
for cmd in docker openssl curl; do command -v "$cmd" >/dev/null 2>&1 && echo "  ok  $cmd" || { echo "  FALTA $cmd"; ok=0; }; done
docker compose version >/dev/null 2>&1 && echo "  ok  docker compose" || { echo "  FALTA docker compose v2"; ok=0; }
command -v go >/dev/null 2>&1 && echo "  ok  go $(go version | awk '{print $3}') (opcional: só para rodar os serviços fora do Docker)"
command -v node >/dev/null 2>&1 && echo "  ok  node $(node --version) (opcional)"
[ "$ok" = 1 ] || exit 1
