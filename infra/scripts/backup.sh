#!/usr/bin/env bash
# Backup de todos os bancos: clientes (pelo provisionador) e painel.
#
# O banco do serviço `dados` (crmia_dados) fica de fora de propósito: é dado
# público, grande e recarregável das fontes. Já o do painel guarda clientes,
# licenças, faturas e o uso da prospecção, e precisa do dump.
set -euo pipefail
cd "$(dirname "$0")/../.."
[ -f .env ] || { echo "sem .env"; exit 1; }
valor() { { grep -E "^$1=" .env || true; } | head -1 | cut -d= -f2- | sed -e 's/^"//' -e 's/"$//'; }
TOKEN="$(valor PROVISIONER_TOKEN)"
DESTINO="${DESTINO:-./backups}"; mkdir -p "$DESTINO"
echo "→ clientes"
for slug in $(docker exec crmia-provisioner wget -qO- --header="Authorization: Bearer $TOKEN" http://127.0.0.1:7898/tenants | python3 -c "import sys,json;print(' '.join(t['slug'] for t in json.load(sys.stdin)))"); do
  echo "  $slug"; docker exec crmia-provisioner wget -qO- --method=POST --header="Authorization: Bearer $TOKEN" --header="Content-Type: application/json" --body-data='{}' "http://127.0.0.1:7898/tenants/$slug/backup" >/dev/null || echo "  falhou: $slug"
done

echo "→ painel"
# O endereço do banco muda conforme o arranjo: PostgreSQL do próprio servidor
# (host.docker.internal, como no Mac mini), serviço `postgres` do Swarm (só na
# rede interna) ou servidor externo. Em vez de adivinhar, usa a mesma
# DATABASE_URL com que o painel está rodando.
URL_BANCO=""
PAINEL="$(docker ps --filter name=crmia-painel --filter name=crmia_painel --format '{{.ID}}' | head -1)"
if [ -n "$PAINEL" ]; then
  URL_BANCO="$(docker inspect "$PAINEL" --format '{{range .Config.Env}}{{println .}}{{end}}' | sed -n 's/^DATABASE_URL=//p' | head -1)"
fi
if [ -z "$URL_BANCO" ]; then
  # Painel parado: monta a URL a partir do .env (PGHOST vazio = banco do servidor).
  host="$(valor PGHOST)"; porta="$(valor PGPORT)"; ssl="$(valor PGSSLMODE)"
  usuario="$(valor PAINEL_DB_USER)"; banco="$(valor PAINEL_DB_NAME)"
  URL_BANCO="postgres://${usuario:-crmia_painel}:$(valor PAINEL_DB_PASSWORD)@${host:-host.docker.internal}:${porta:-5432}/${banco:-crmia_painel}?sslmode=${ssl:-disable}"
fi
HOST_BANCO="$(printf '%s' "$URL_BANCO" | sed -E 's|^[a-z]+://([^@/]*@)?([^:/?]+).*$|\2|')"
# De onde o dump consegue alcançar o banco:
#   host.docker.internal → rede padrão do Docker (a crmia_interna não tem saída para o host);
#   nome sem ponto (ex.: postgres, no Swarm) → rede interna da plataforma;
#   qualquer outro endereço → rede padrão, com saída para fora.
case "$HOST_BANCO" in
  host.docker.internal) REDE=(--add-host=host.docker.internal:host-gateway) ;;
  *.*|*:*) REDE=() ;;
  *) REDE=(--network crmia_interna) ;;
esac
ARQUIVO="$DESTINO/painel-$(date +%Y%m%d-%H%M%S).sql.gz"
# A URL (com a senha) vai pelo ambiente do container, nunca pela linha de comando.
if URL_BANCO="$URL_BANCO" docker run --rm ${REDE[@]+"${REDE[@]}"} -e URL_BANCO postgres:16-alpine \
     sh -c 'pg_dump --no-owner --dbname="$URL_BANCO"' | gzip > "$ARQUIVO.parcial" && [ "$(gzip -dc "$ARQUIVO.parcial" | head -c 64 | wc -c)" -gt 0 ]; then
  mv "$ARQUIVO.parcial" "$ARQUIVO"
  echo "  $ARQUIVO ($(du -h "$ARQUIVO" | cut -f1))"
else
  rm -f "$ARQUIVO.parcial"
  echo "✗ o dump do painel falhou (banco em $HOST_BANCO); nenhum arquivo foi gravado" >&2
  exit 1
fi
echo "✓ backups em $DESTINO e no volume crmia_backups"
