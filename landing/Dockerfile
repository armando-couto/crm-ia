# Landing do CRM IA (site estático pré-renderizado). Planos e preços vêm do
# painel em tempo de execução (/painel/api/planos): mudar preço não exige rebuild.
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json ./
RUN npm install --no-audit --no-fund
COPY . .
ARG DOMINIO=crmia.com.br
ARG EMAIL_COMERCIAL=contato@crmia.com.br
ARG WHATSAPP=
ENV VITE_DOMINIO=$DOMINIO VITE_EMAIL_COMERCIAL=$EMAIL_COMERCIAL VITE_WHATSAPP=$WHATSAPP NODE_ENV=production
RUN npm run build:rapido

FROM nginxinc/nginx-unprivileged:1.27-alpine
ARG DOMINIO=crmia.com.br
ENV DOMINIO=$DOMINIO
COPY --chown=nginx:nginx nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build --chown=nginx:nginx /app/dist /usr/share/nginx/html
COPY --chown=nginx:nginx docker-entrypoint.d/10-robots-sitemap.sh /docker-entrypoint.d/10-robots-sitemap.sh
USER root
RUN chmod +x /docker-entrypoint.d/10-robots-sitemap.sh && chown -R nginx:nginx /usr/share/nginx/html
USER nginx
EXPOSE 7894
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD wget -qO- http://127.0.0.1:7894/healthz || exit 1
