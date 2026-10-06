#!/bin/sh
# Gera robots.txt e sitemap.xml com o domínio real (o build é genérico).
set -eu
D="${DOMINIO:-localhost}"
DATA="$(date -u +%Y-%m-%d)"
cat > /usr/share/nginx/html/robots.txt <<FIM
User-agent: *
Allow: /
Disallow: /painel
Sitemap: https://${D}/sitemap.xml
FIM
cat > /usr/share/nginx/html/sitemap.xml <<FIM
<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://${D}/</loc><lastmod>${DATA}</lastmod><changefreq>weekly</changefreq><priority>1.0</priority></url>
</urlset>
FIM
