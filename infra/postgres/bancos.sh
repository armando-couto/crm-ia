#!/bin/sh
# Cria (ou sincroniza) os bancos da plataforma e seus donos, a partir do
# ambiente já exportado pelo entrypoint: o do painel (com pgcrypto) e o da
# base pública da prospecção, do serviço `dados`. Idempotente.
set -eu
i=0
until pg_isready -h 127.0.0.1 -p 5432 -q; do
  i=$((i + 1)); [ "$i" -gt 300 ] && { echo "crmia: PostgreSQL não respondeu em 5 min" >&2; exit 1; }
  sleep 1
done
export PGPASSWORD="$POSTGRES_PASSWORD"
psql_admin() { psql -h 127.0.0.1 -p 5432 -U "$POSTGRES_USER" -v ON_ERROR_STOP=1 -qAt "$@"; }
# preparar <banco> <dono> <senha> <variável da senha> [extensão obrigatória]
preparar() {
  banco="$1"; usuario="$2"; senha="$3"; extensao="${5:-}"
  [ -n "$senha" ] || { echo "crmia:   $banco sem senha ($4), pulando"; return 0; }
  senha_sql=$(printf '%s' "$senha" | sed "s/'/''/g")
  psql_admin -d postgres -c "DO \$\$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='${usuario}') THEN CREATE ROLE ${usuario} LOGIN PASSWORD '${senha_sql}'; ELSE ALTER ROLE ${usuario} LOGIN PASSWORD '${senha_sql}'; END IF; END \$\$;"
  if [ "$(psql_admin -d postgres -c "SELECT 1 FROM pg_database WHERE datname='${banco}'")" != "1" ]; then
    psql_admin -d postgres -c "CREATE DATABASE ${banco} OWNER ${usuario} ENCODING 'UTF8'"
  else
    psql_admin -d postgres -c "ALTER DATABASE ${banco} OWNER TO ${usuario}"
  fi
  psql_admin -d postgres -c "REVOKE CONNECT ON DATABASE ${banco} FROM PUBLIC"
  psql_admin -d postgres -c "GRANT CONNECT ON DATABASE ${banco} TO ${usuario}"
  if [ -n "$extensao" ]; then psql_admin -d "$banco" -c "CREATE EXTENSION IF NOT EXISTS ${extensao}"; fi
  echo "crmia:   ok $banco (dono $usuario)"
}
# extensao_opcional <banco> <extensão>: cria se a imagem tiver; se não tiver,
# avisa e segue (o serviço `dados` funciona sem extensão nenhuma).
extensao_opcional() {
  if psql_admin -d "$1" -c "CREATE EXTENSION IF NOT EXISTS $2" >/dev/null 2>&1; then return 0; fi
  echo "crmia:   aviso: extensão $2 indisponível; $1 segue sem ela"
}
echo "crmia: preparando o banco do painel"
preparar "${PAINEL_DB_NAME:-crmia_painel}" "${PAINEL_DB_USER:-crmia_painel}" "${PAINEL_DB_PASSWORD:-}" PAINEL_DB_PASSWORD pgcrypto
echo "crmia: banco do painel pronto"
if [ -n "${DADOS_DB_PASSWORD:-}" ]; then
  echo "crmia: preparando o banco da base pública (serviço dados)"
  preparar "${DADOS_DB_NAME:-crmia_dados}" "${DADOS_DB_USER:-crmia_dados}" "$DADOS_DB_PASSWORD" DADOS_DB_PASSWORD
  # pg_prewarm só aquece a base depois de cada carga; precisa de administrador.
  extensao_opcional "${DADOS_DB_NAME:-crmia_dados}" pg_prewarm
  echo "crmia: banco da base pública pronto"
else
  echo "crmia: DADOS_DB_PASSWORD vazia; banco da base pública não criado (prospecção desabilitada)"
fi
