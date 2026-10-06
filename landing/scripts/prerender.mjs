// Gera o HTML final da landing no build: o Vue renderiza a página inteira e o
// resultado vai para o index.html, junto dos dados estruturados (JSON-LD).
// Assim o buscador e o compartilhamento em redes enxergam o conteúdo sem
// executar JavaScript.
import { readFileSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'

const raiz = resolve(import.meta.dirname, '..')
const dominio = process.env.VITE_DOMINIO || 'crmia.com.br'
const site = `https://${dominio}`

const { renderizar, criarFaq, grupos } = await import(resolve(raiz, 'dist-ssr/entry-server.js'))
const html = await renderizar()
const faq = criarFaq(dominio)
const dados = [
  {
    '@context': 'https://schema.org',
    '@type': 'SoftwareApplication',
    name: 'CRM IA',
    applicationCategory: 'BusinessApplication',
    applicationSubCategory: 'CRM',
    operatingSystem: 'Web',
    url: site + '/',
    inLanguage: 'pt-BR',
    description: 'CRM na nuvem com ambiente isolado por empresa: contatos, negócios em kanban, sequências de e-mail, automações, caixa de entrada e relatórios, com e-mail no domínio do cliente.',
    offers: { '@type': 'AggregateOffer', priceCurrency: 'BRL', lowPrice: '99.00', offerCount: 3, availability: 'https://schema.org/InStock' },
    publisher: { '@type': 'Organization', name: 'CRM IA', url: site + '/' },
    featureList: grupos.flatMap((g) => g.itens.map((i) => i.t))
  },
  { '@context': 'https://schema.org', '@type': 'FAQPage', mainEntity: faq.map(({ p, r }) => ({ '@type': 'Question', name: p, acceptedAnswer: { '@type': 'Answer', text: r } })) }
]
const caminho = resolve(raiz, 'dist/index.html')
let pagina = readFileSync(caminho, 'utf-8')
pagina = pagina
  .replace('<div id="app"></div>', `<div id="app">${html}</div>`)
  .replaceAll('__DOMINIO__', dominio)
  .replace('</head>', `  <script type="application/ld+json">${JSON.stringify(dados)}</script>\n  </head>`)
writeFileSync(caminho, pagina)
console.log(`landing pré-renderizada: ${(Buffer.byteLength(pagina) / 1024).toFixed(0)} kB, ${faq.length} perguntas no JSON-LD`)
