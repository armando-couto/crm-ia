#!/usr/bin/env bash
# =============================================================================
# CRM IA — implanta ou atualiza a plataforma NESTE servidor (rodar de dentro
# do clone do projeto). Serve para a primeira vez e para cada nova versão:
#
#   ./atualiza_projeto.sh                 # git pull + build + sobe + atualiza clientes
#   ./atualiza_projeto.sh --sem-pull      # usa o código como está
#   ./atualiza_projeto.sh --sem-clientes  # atualiza só a plataforma
#
# Na primeira execução cria o .env (segredos sorteados), usa o IP do servidor
# como endereço e prepara o banco crmia_painel no PostgreSQL deste servidor.
# Variáveis opcionais na primeira vez: ENDERECO (IP ou domínio),
# PGADMIN_USER / PGADMIN_PASSWORD (superusuário do PostgreSQL, padrão postgres).
# =============================================================================
set -euo pipefail
cd "$(dirname "$0")"

PULL=1; CLIENTES=1
for a in "$@"; do case "$a" in
  --sem-pull) PULL=0 ;; --sem-clientes) CLIENTES=0 ;;
  -h|--help) sed -n 2,14p "$0"; exit 0 ;;
  *) echo "opção desconhecida: $a"; exit 1 ;;
esac; done

COMPOSE=(docker compose -f docker-compose.yml -f docker-compose.servidor.yml)
valor() { { grep -E "^$1=" .env || true; } | head -1 | cut -d= -f2- | sed -e 's/^"//' -e 's/"$//'; }
passo() { echo; echo "→ $*"; }

command -v docker >/dev/null || { echo "Docker não instalado"; exit 1; }
docker compose version >/dev/null 2>&1 || { echo "Falta o plugin docker compose v2"; exit 1; }
docker info >/dev/null 2>&1 || { echo "Sem acesso ao Docker: rode com sudo ou adicione o usuário ao grupo docker"; exit 1; }

if [ "$PULL" = 1 ]; then
  passo "atualizando o código (git pull)"
  git pull --ff-only
fi
VERSAO="$(git describe --tags --always 2>/dev/null | sed 's/^v//')"
[ -n "$VERSAO" ] || VERSAO="$(date +%Y%m%d%H%M)"
echo "   versão: $VERSAO"

if [ ! -f .env ]; then
  passo "primeira execução: criando o .env"
  ENDERECO="${ENDERECO:-$(hostname -I 2>/dev/null | awk '{print $1}')}"
  : "${ENDERECO:?não descobri o IP; rode com ENDERECO=192.168.1.134 ./atualiza_projeto.sh}"
  bash infra/scripts/gerar-env.sh .env >/dev/null
  sed -i \
    -e "s|^DOMAIN=.*|DOMAIN=$ENDERECO|" \
    -e "s|^ACME_EMAIL=.*|ACME_EMAIL=ti@crmia.local|" \
    -e "s|^PGHOST=.*|PGHOST=host.docker.internal|" \
    -e "s|^PGSSLMODE=.*|PGSSLMODE=disable|" \
    -e "s|^FIXPAY_SIMULADO=.*|FIXPAY_SIMULADO=true|" \
    -e "s|^PGADMIN_USER=.*|PGADMIN_USER=${PGADMIN_USER:-postgres}|" \
    -e "s|^PGADMIN_PASSWORD=.*|PGADMIN_PASSWORD=${PGADMIN_PASSWORD:-postgres}|" .env
  echo "   endereço: https://$ENDERECO  (edite o .env para trocar)"
else
  bash infra/scripts/gerar-env.sh .env --completar >/dev/null   # acrescenta chaves novas
fi

passo "garantindo o banco crmia_painel no PostgreSQL do servidor"
bash infra/scripts/preparar-postgres-dev.sh

passo "construindo as imagens"
docker build -q -t crmia/suspenso:latest ./infra/suspenso >/dev/null
docker build -q --build-arg VERSAO="$VERSAO" -t "crmia/app:$VERSAO" -t crmia/app:latest ./app >/dev/null
"${COMPOSE[@]}" build -q

passo "subindo a plataforma"
"${COMPOSE[@]}" up -d --remove-orphans

passo "aguardando o painel"
for i in $(seq 1 40); do
  docker exec crmia-painel /painel -healthcheck >/dev/null 2>&1 && break
  [ "$i" = 40 ] && { echo "o painel não respondeu; veja: docker logs crmia-painel"; exit 1; }
  sleep 3
done
echo "   ok"

if [ "$CLIENTES" = 1 ]; then
  passo "registrando a versão $VERSAO e atualizando os clientes com auto-atualizar"
  TOKEN="$(valor PROVISIONER_TOKEN)"
  # O painel é distroless (sem shell): a chamada sai de um curl na rede interna.
  docker run --rm --network crmia_interna curlimages/curl:8.10.1 -sS --max-time 600 \
    -X POST http://painel:7894/painel/api/interno/versoes \
    -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
    -d "{\"tag\":\"$VERSAO\",\"descricao\":\"atualiza_projeto.sh\",\"estavel\":true,\"padrao\":true,\"aplicar\":true}" \
    || echo "   aviso: cadastre a versão $VERSAO em Painel → Versões"
  echo
fi

docker image prune -f >/dev/null 2>&1 || true
END="$(valor DOMAIN)"
echo
echo "✓ CRM IA $VERSAO no ar"
echo "  site:    https://$END/"
echo "  painel:  https://$END/painel   ($(valor PAINEL_ADMIN_EMAIL) / $(valor PAINEL_ADMIN_SENHA))"
echo "  clientes: https://$END/<slug>"
