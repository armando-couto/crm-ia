# PostgreSQL da plataforma (banco do painel e banco da base pública da
# prospecção), usado pelo stack Swarm (stack-portainer.yml). Recebe
# PGADMIN_*/PAINEL_DB_*/DADOS_DB_* pelo ambiente e, a cada subida, garante os
# bancos e seus donos (senha trocada no .env também é atendida). Opcionalmente lê um /.env.production montado como segredo.
FROM postgres:16-alpine
COPY entrypoint.sh /usr/local/bin/crmia-entrypoint.sh
COPY bancos.sh /usr/local/bin/crmia-bancos.sh
RUN chmod +x /usr/local/bin/crmia-entrypoint.sh /usr/local/bin/crmia-bancos.sh
ENTRYPOINT ["crmia-entrypoint.sh"]
CMD ["postgres"]
