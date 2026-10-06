#!/usr/bin/env bash
# Atalho de operação dos ambientes, falando com o provisionador pela rede interna.
#   tenant.sh criar <slug> "<nome>" <cnpj> [versao] | status <slug> | atualizar <slug> [versao]
#   tenant.sh suspender <slug> | reativar <slug> | backup <slug> | remover <slug>
# Em produção prefira o painel: ele registra quem fez o quê.
set -euo pipefail
cd "$(dirname "$0")/../.."
[ -f .env ] || { echo "sem .env — rode make env"; exit 1; }
TOKEN="$(grep -E '^PROVISIONER_TOKEN=' .env | cut -d= -f2-)"
chamar() {
  local metodo="$1" caminho="$2" corpo="${3:-}"
  docker exec crmia-provisioner wget -qO- --method="$metodo" --header="Authorization: Bearer $TOKEN" --header="Content-Type: application/json" ${corpo:+--body-data="$corpo"} "http://127.0.0.1:7898$caminho" || true
  echo
}
acao="${1:-}"; slug="${2:-}"
case "$acao" in
  criar) nome="${3:?informe o nome}"; cnpj="${4:?informe o CNPJ}"; versao="${5:-latest}"
    chamar POST /tenants "{\"slug\":\"$slug\",\"nome\":\"$nome\",\"cnpj\":\"$cnpj\",\"versao\":\"$versao\",\"usuarios_max\":5}" ;;
  status) chamar GET "/tenants/$slug" ;;
  atualizar) chamar POST "/tenants/$slug/atualizar" "{\"versao\":\"${3:-}\"}" ;;
  suspender) chamar POST "/tenants/$slug/suspender" ;;
  reativar) chamar POST "/tenants/$slug/reativar" ;;
  backup) chamar POST "/tenants/$slug/backup" ;;
  remover) read -r -p "Remover '$slug' (backup antes). Digite o slug para confirmar: " conf
    [ "$conf" = "$slug" ] || { echo "cancelado"; exit 1; }
    chamar DELETE "/tenants/$slug" "{\"confirmar\":\"$slug\"}" ;;
  *) sed -n '2,5p' "$0"; exit 1 ;;
esac
