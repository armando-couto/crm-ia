#!/usr/bin/env bash
# Implanta a plataforma num servidor Docker da rede local, com o banco do
# painel no PostgreSQL do próprio servidor.
#   SERVIDOR=usuario@192.168.1.134 bash infra/scripts/implantar-servidor.sh
# Variáveis opcionais: DESTINO (/opt/crmia), PGADMIN_USER/PGADMIN_PASSWORD
# (superusuário do PostgreSQL do servidor, padrão postgres/postgres).
set -euo pipefail
cd "$(dirname "$0")/../.."
: "${SERVIDOR:?informe SERVIDOR=usuario@ip}"
DESTINO="${DESTINO:-/opt/crmia}"
IP="${SERVIDOR#*@}"
echo "→ copiando o projeto para $SERVIDOR:$DESTINO"
ssh "$SERVIDOR" "sudo mkdir -p $DESTINO && sudo chown \$(id -u):\$(id -g) $DESTINO"
rsync -az --delete --exclude .git --exclude node_modules --exclude dist --exclude dist-ssr \
  --exclude .env --exclude 'backups' ./ "$SERVIDOR:$DESTINO/"
ssh -t "$SERVIDOR" "cd $DESTINO && IP=$IP PGADMIN_USER=${PGADMIN_USER:-postgres} PGADMIN_PASSWORD=${PGADMIN_PASSWORD:-postgres} bash -s" <<'REMOTO'
set -euo pipefail
if [ ! -f .env ]; then
  bash infra/scripts/gerar-env.sh .env >/dev/null
  sed -i "s|^DOMAIN=.*|DOMAIN=$IP|;s|^ACME_EMAIL=.*|ACME_EMAIL=ti@crmia.local|;s|^PGHOST=.*|PGHOST=host.docker.internal|;s|^PGSSLMODE=.*|PGSSLMODE=disable|;s|^FIXPAY_SIMULADO=.*|FIXPAY_SIMULADO=true|;s|^PGADMIN_USER=.*|PGADMIN_USER=$PGADMIN_USER|" .env
  sed -i "s|^PGADMIN_PASSWORD=.*|PGADMIN_PASSWORD=$PGADMIN_PASSWORD|" .env
fi
echo "→ preparando o banco crmia_painel no PostgreSQL do servidor"
PGADMIN_USER=$PGADMIN_USER PGADMIN_PASSWORD=$PGADMIN_PASSWORD bash infra/scripts/preparar-postgres-dev.sh
echo "→ construindo imagens e subindo"
docker build -q -t crmia/suspenso:latest ./infra/suspenso >/dev/null
docker build -q --build-arg VERSAO=0.2.0 -t crmia/app:0.2.0 -t crmia/app:latest ./app >/dev/null
docker compose -f docker-compose.yml -f docker-compose.servidor.yml up -d --build --remove-orphans
sleep 10
curl -sk https://127.0.0.1/painel/saude -H "Host: $IP" && echo
echo "✓ https://$IP/   painel: https://$IP/painel  ($(grep ^PAINEL_ADMIN_EMAIL= .env | cut -d= -f2) / $(grep ^PAINEL_ADMIN_SENHA= .env | cut -d= -f2))"
REMOTO
