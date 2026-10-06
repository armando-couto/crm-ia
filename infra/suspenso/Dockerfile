# Página estática de "acesso suspenso": sobe no lugar do app do cliente
# enquanto a assinatura não está ativa.
FROM nginxinc/nginx-unprivileged:1.27-alpine
COPY --chown=nginx:nginx nginx.conf /etc/nginx/conf.d/default.conf
COPY --chown=nginx:nginx index.html /usr/share/nginx/html/index.html
EXPOSE 8080
