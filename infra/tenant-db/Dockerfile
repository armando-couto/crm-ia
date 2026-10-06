# Banco de cada cliente: o postgres oficial, sem mudança — só republicado no
# registro da plataforma para o provisionador baixar com a credencial que já
# usa (o Docker Hub corta downloads anônimos por IP). NÃO embute .env: o banco
# de um cliente não carrega segredos da plataforma.
FROM postgres:16-alpine
LABEL org.opencontainers.image.description="CRM IA — PostgreSQL do ambiente de cada cliente (espelho de postgres:16-alpine)"
