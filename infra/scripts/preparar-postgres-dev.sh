#!/usr/bin/env bash
# Cria (ou sincroniza) os bancos da plataforma no PostgreSQL que JÁ roda na
# máquina (padrão 127.0.0.1:5432, postgres/postgres): o do painel
# (crmia_painel) e o da base pública da prospecção, do serviço `dados`
# (crmia_dados). Apesar do nome, é o mesmo script usado no servidor
# (atualiza_projeto.sh, implantar-servidor.sh). Idempotente; a senha do .env é
# a que vale e é reaplicada a cada execução.
set -euo pipefail
cd "$(dirname "$0")/../.."
[ -f .env ] || { echo "sem .env — rode make env"; exit 1; }
valor() { { grep -E "^$1=" .env || true; } | head -1 | cut -d= -f2- | sed -e 's/^"//' -e 's/"$//'; }
HOST="${DEV_PGHOST_LOCAL:-127.0.0.1}"; PORT="${DEV_PGPORT:-5432}"
ADMIN="${PGADMIN_USER:-$(valor PGADMIN_USER)}"; ADMIN="${ADMIN:-postgres}"
ADMIN_SENHA="${PGADMIN_PASSWORD:-$(valor PGADMIN_PASSWORD)}"; ADMIN_SENHA="${ADMIN_SENHA:-postgres}"
PAINEL_SENHA="$(valor PAINEL_DB_PASSWORD)"; PAINEL_SENHA="${PAINEL_SENHA:-dev}"
DADOS_BANCO="$(valor DADOS_DB_NAME)"; DADOS_BANCO="${DADOS_BANCO:-crmia_dados}"
DADOS_USUARIO="$(valor DADOS_DB_USER)"; DADOS_USUARIO="${DADOS_USUARIO:-crmia_dados}"
DADOS_SENHA="$(valor DADOS_DB_PASSWORD)"; DADOS_SENHA="${DADOS_SENHA:-dev}"
if command -v psql >/dev/null 2>&1; then
  PSQL=(env PGPASSWORD="$ADMIN_SENHA" psql -h "$HOST" -p "$PORT" -U "$ADMIN" -v ON_ERROR_STOP=1 -qAt)
else
  PSQL=(docker run --rm -i -e PGPASSWORD="$ADMIN_SENHA" --add-host=host.docker.internal:host-gateway postgres:16-alpine psql -h host.docker.internal -p "$PORT" -U "$ADMIN" -v ON_ERROR_STOP=1 -qAt)
fi
if ! "${PSQL[@]}" -d postgres -c "select 1" >/dev/null 2>&1 && [ "$ADMIN_SENHA" != "postgres" ]; then
  ADMIN_SENHA=postgres
  for i in "${!PSQL[@]}"; do case "${PSQL[$i]}" in PGPASSWORD=*) PSQL[$i]="PGPASSWORD=postgres";; esac; done
fi
"${PSQL[@]}" -d postgres -c "select 1" >/dev/null || { echo "não consegui falar com o PostgreSQL em $HOST:$PORT como $ADMIN"; exit 1; }

# preparar <banco> <papel> <senha>: papel com a senha do .env e banco dele.
preparar() {
  local banco="$1" papel="$2" aspa="'" senha sql
  senha="${3//$aspa/$aspa$aspa}"
  # Os nomes entram no SQL sem aspas: só identificadores simples.
  [[ "$banco" =~ ^[a-z_][a-z0-9_]*$ && "$papel" =~ ^[a-z_][a-z0-9_]*$ ]] || { echo "nome de banco ou papel inválido: $banco / $papel"; exit 1; }
  sql="$(printf "DO \$\$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='%s') THEN CREATE ROLE %s LOGIN PASSWORD '%s'; ELSE ALTER ROLE %s LOGIN PASSWORD '%s'; END IF; END \$\$;" "$papel" "$papel" "$senha" "$papel" "$senha")"
  "${PSQL[@]}" -d postgres -c "$sql"
  if [ "$("${PSQL[@]}" -d postgres -c "select 1 from pg_database where datname='$banco'")" != "1" ]; then
    "${PSQL[@]}" -d postgres -c "CREATE DATABASE $banco OWNER $papel ENCODING 'UTF8'"; echo "  banco $banco criado"
  else
    "${PSQL[@]}" -d postgres -c "ALTER DATABASE $banco OWNER TO $papel"
  fi
}

# extensao_opcional <banco> <extensão>: cria como administrador se a instalação
# tiver; se não tiver, segue sem ela (nunca derruba a preparação).
extensao_opcional() {
  if "${PSQL[@]}" -d "$1" -c "CREATE EXTENSION IF NOT EXISTS $2" >/dev/null 2>&1; then
    return 0
  fi
  echo "  aviso: extensão $2 indisponível neste PostgreSQL; o banco $1 segue sem ela"
}

preparar crmia_painel crmia_painel "$PAINEL_SENHA"
"${PSQL[@]}" -d crmia_painel -c "CREATE EXTENSION IF NOT EXISTS pgcrypto" >/dev/null
echo "→ banco crmia_painel pronto em $HOST:$PORT"

# Base pública da prospecção. O serviço não depende de extensão nenhuma;
# pg_prewarm só aquece a base depois de cada troca e exige administrador para
# ser criada, por isso é feita aqui.
preparar "$DADOS_BANCO" "$DADOS_USUARIO" "$DADOS_SENHA"
extensao_opcional "$DADOS_BANCO" pg_prewarm
echo "→ banco $DADOS_BANCO pronto em $HOST:$PORT"
