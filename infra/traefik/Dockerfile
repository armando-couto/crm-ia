# Traefik do CRM IA com a configuração dentro da imagem. ACME_EMAIL e
# ACME_CA_SERVER (staging do Let's Encrypt para ensaios) vêm como build-args.
FROM traefik:v3.7
ARG ACME_EMAIL=ti@crmia.com.br
ARG ACME_CA_SERVER=https://acme-v02.api.letsencrypt.org/directory
COPY traefik.yml /etc/traefik/traefik.yml
COPY dynamic/ /etc/traefik/dynamic/
RUN mkdir -p /letsencrypt \
 && sed -i "s|__ACME_EMAIL__|${ACME_EMAIL}|; s|__ACME_CA_SERVER__|${ACME_CA_SERVER}|" /etc/traefik/traefik.yml
