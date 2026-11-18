#!/usr/bin/env bash
# Renova o túnel temporário da Cloudflare (trycloudflare.com) e aponta os
# links do sistema para o endereço novo. Não reconstrói imagens.
#   ./tunel.sh
# Para um endereço que nunca muda, use o túnel fixo (README → Cloudflare).
set -euo pipefail
cd "$(dirname "$0")"
[ -f .env ] || { echo "sem .env — rode ./atualiza_projeto.sh primeiro"; exit 1; }
valor() { { grep -E "^$1=" .env || true; } | head -1 | cut -d= -f2- | sed -e 's/^"//' -e 's/"$//'; }
DOM="$(valor DOMAIN)"; PORTA="$(valor HTTPS_PORT)"; PORTA="${PORTA:-443}"

echo "→ recriando o túnel"
docker rm -f crmia-tunel >/dev/null 2>&1 || true
docker run -d --name crmia-tunel --restart unless-stopped --add-host=host.docker.internal:host-gateway \
  cloudflare/cloudflared:2025.10.0 tunnel --no-autoupdate --url "https://host.docker.internal:$PORTA" \
  --no-tls-verify --http-host-header "$DOM" --origin-server-name "$DOM" >/dev/null

URL=""
for i in $(seq 1 30); do
  URL="$(docker logs crmia-tunel 2>&1 | grep -o 'https://[a-z0-9-]*\.trycloudflare\.com' | tail -1 || true)"
  [ -n "$URL" ] && break; sleep 2
done
[ -n "$URL" ] || { echo "o túnel não informou o endereço; veja: docker logs crmia-tunel"; exit 1; }

echo "→ apontando os links para $URL"
if grep -q '^URL_PUBLICA=' .env; then
  sed -i.bak "s|^URL_PUBLICA=.*|URL_PUBLICA=$URL|" .env && rm -f .env.bak
else
  echo "URL_PUBLICA=$URL" >> .env
fi
docker compose -f docker-compose.yml -f docker-compose.servidor.yml up -d --no-build painel provisioner >/dev/null

echo "→ testando"
for i in $(seq 1 20); do
  [ "$(curl -s -o /dev/null -w '%{http_code}' -m10 "$URL/painel/saude" || true)" = 200 ] && break; sleep 3
done
echo
echo "✓ endereço atual: $URL"
echo "  painel: $URL/painel"
