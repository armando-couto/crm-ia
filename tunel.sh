#!/usr/bin/env bash
# Renova o túnel temporário da Cloudflare (trycloudflare.com) e aponta os
# links do sistema para o endereço novo. Não reconstrói imagens.
#   ./tunel.sh               renova agora (o endereço muda)
#   ./tunel.sh --verificar   só renova se o endereço atual não responder
#   ./tunel.sh --agendar     instala a verificação automática a cada 10 min
#                            (launchd no macOS, cron no Linux)
#   ./tunel.sh --desagendar  remove a verificação automática
# Para um endereço que nunca muda, use o túnel fixo (README → Cloudflare).
set -euo pipefail
cd "$(dirname "$0")"
export PATH="/usr/local/bin:/opt/homebrew/bin:/Applications/Docker.app/Contents/Resources/bin:$PATH"
MODO="${1:-}"
AQUI="$(pwd)"
ROTULO="br.crmia.tunel"
PLIST="$HOME/Library/LaunchAgents/$ROTULO.plist"
LOG="$AQUI/tunel.log"

agendar() {
  if [ "$(uname)" = Darwin ]; then
    mkdir -p "$HOME/Library/LaunchAgents"
    cat > "$PLIST" <<PL
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>$ROTULO</string>
  <key>ProgramArguments</key><array><string>/bin/bash</string><string>$AQUI/tunel.sh</string><string>--verificar</string></array>
  <key>StartInterval</key><integer>600</integer>
  <key>RunAtLoad</key><true/>
  <key>StandardOutPath</key><string>$LOG</string>
  <key>StandardErrorPath</key><string>$LOG</string>
</dict></plist>
PL
    launchctl bootout "gui/$(id -u)/$ROTULO" >/dev/null 2>&1 || true
    launchctl bootstrap "gui/$(id -u)" "$PLIST" 2>/dev/null || launchctl load -w "$PLIST"
  else
    ( crontab -l 2>/dev/null | grep -v 'tunel.sh --verificar'; echo "*/10 * * * * /bin/bash $AQUI/tunel.sh --verificar >> $LOG 2>&1" ) | crontab -
  fi
  echo "✓ verificação automática a cada 10 min (log em $LOG)"
}
desagendar() {
  if [ "$(uname)" = Darwin ]; then
    launchctl bootout "gui/$(id -u)/$ROTULO" >/dev/null 2>&1 || launchctl unload -w "$PLIST" >/dev/null 2>&1 || true
    rm -f "$PLIST"
  else
    crontab -l 2>/dev/null | grep -v 'tunel.sh --verificar' | crontab - || true
  fi
  echo "✓ verificação automática removida"
}
case "$MODO" in
  --agendar) agendar; exit 0 ;;
  --desagendar) desagendar; exit 0 ;;
  ""|--verificar) ;;
  *) sed -n 2,9p "$0"; exit 1 ;;
esac
[ -f .env ] || { echo "sem .env — rode ./atualiza_projeto.sh primeiro"; exit 1; }
valor() { { grep -E "^$1=" .env || true; } | head -1 | cut -d= -f2- | sed -e 's/^"//' -e 's/"$//'; }
DOM="$(valor DOMAIN)"; PORTA="$(valor HTTPS_PORT)"; PORTA="${PORTA:-443}"

if grep -qE '^CLOUDFLARE_TUNNEL_TOKEN=.+' .env; then
  [ "$MODO" = --verificar ] || echo "túnel fixo configurado (CLOUDFLARE_TUNNEL_TOKEN): nada a renovar"
  exit 0
fi

if [ "$MODO" = --verificar ]; then
  ATUAL="$(valor URL_PUBLICA)"
  if [[ "$ATUAL" == *trycloudflare.com* ]] && docker ps --format '{{.Names}}' | grep -qx crmia-tunel; then
    for i in 1 2 3; do
      [ "$(curl -s -o /dev/null -w '%{http_code}' -m15 "$ATUAL/painel/saude" || true)" = 200 ] && exit 0
      sleep 10
    done
  fi
  echo "$(date '+%F %T') endereço fora do ar ($ATUAL); renovando"
fi

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
echo "$(date '+%F %T') ✓ endereço atual: $URL"
echo "  painel: $URL/painel"
