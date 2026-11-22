#!/usr/bin/env bash
# Operação direta do serviço `dados` (base pública da prospecção), pela rede
# interna da plataforma:
#   bash infra/scripts/dados.sh situacao           # referências, totais e carga em curso
#   bash infra/scripts/dados.sh carga [cnpj|pgfn|tudo]
set -euo pipefail
cd "$(dirname "$0")/../.."
[ -f .env ] || { echo "sem .env"; exit 1; }
TOKEN="$({ grep -E '^DADOS_TOKEN=' .env || true; } | head -1 | cut -d= -f2- | sed -e 's/^"//' -e 's/"$//')"
[ -n "$TOKEN" ] || { echo "DADOS_TOKEN vazio no .env (rode: bash infra/scripts/gerar-env.sh .env --completar)"; exit 1; }
# O token vai pelo ambiente do container, não pela linha de comando.
chamar() { # chamar <método> <caminho> [corpo]
  DADOS_TOKEN="$TOKEN" docker run --rm --network crmia_interna -e DADOS_TOKEN --entrypoint sh curlimages/curl:8.10.1 -c \
    'curl -sS --max-time 20 -X "$1" -H "Authorization: Bearer $DADOS_TOKEN" -H "Content-Type: application/json" ${3:+-d "$3"} "http://dados:7896$2"' sh "$@"
  echo
}
case "${1:-situacao}" in
  situacao) chamar GET /v1/situacao ;;
  carga)
    fonte="${2:-tudo}"
    case "$fonte" in cnpj|pgfn|tudo) ;; *) echo "fonte inválida: $fonte (use cnpj, pgfn ou tudo)"; exit 1 ;; esac
    chamar POST /v1/cargas "{\"fonte\":\"$fonte\"}" ;;
  *) echo "uso: dados.sh situacao | carga [cnpj|pgfn|tudo]"; exit 1 ;;
esac
