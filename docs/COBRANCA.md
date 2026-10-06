# Cobrança das licenças

O cliente paga uma **assinatura mensal no cartão de crédito**, por grupo pagador (CNPJ). O processador da recorrência é um serviço de terceiros contratado pela CRM IA; **o cliente final nunca o vê**: a página de cadastro do cartão, os e-mails e as telas do CRM só mostram a marca CRM IA. Nos arquivos de configuração ele aparece como `FIXPAY_*` porque são as credenciais da própria CRM IA junto ao processador.

## Objetos

| Objeto | Papel |
|---|---|
| `planos` | faixa de usuários, preço mensal, destaque na landing |
| `licencas` | grupo × plano × unidades × periodicidade (mensal/anual); `cobranca` = `recorrente` ou `manual` |
| `faturas` | uma por competência da licença; `forma` = recorrência ou manual; `referencia` = id da cobrança no processador |
| `licencas.pagamento_status` | `aguardando_cartao` → `ativo` / `recusado` / `cancelado`; `manual` para licenças manuais |
| `cobranca_eventos` | tudo que chegou do processador (relatório, webhook) e tudo que o painel enviou |

## Fluxo da licença recorrente

1. **Contratar** (`ContratarLicenca`): o painel garante um *pagador* no processador para o grupo (`GarantirPagador`), cria a *assinatura* com o valor calculado (`CalcularValor` = preço do plano × unidades, com desconto anual se houver), guarda `assinatura_id`/`assinatura_token` cifrados e gera um `codigo_checkout`. Abre a primeira fatura.
2. **Link do cartão**: `https://<dominio>/painel/assinar/<codigo>`. O comercial envia pelo botão "enviar link" (e-mail com a marca CRM IA) ou copia o endereço. A página pede CPF/CNPJ, titular, número, validade, CVV e endereço, e envia ao painel.
3. **Cadastrar cartão** (`CadastrarCartao`): o painel valida (Luhn, validade, bandeira) e repassa ao gateway de integração do processador. Aprovado → `RegistrarCartao` (final, bandeira, data), `pagamento_status=ativo` e a primeira fatura em aberto é baixada. Recusado → mensagem genérica ao cliente, detalhe em `cobranca_eventos`.
4. **Renovações**: o processador cobra todo mês. O painel descobre o pagamento de três formas, todas idempotentes (`RegistrarPagamento` ignora fatura já paga):
   * **Conferente** (job a cada 30 min): `billingreportsignatures` por assinatura; casa pela competência.
   * **Webhook** `POST /painel/api/webhooks/recorrencia` (`PagamentoConfirmado`).
   * **Baixa manual** na tela Faturas (também serve para PIX/boleto negociado à parte).
5. **Baixa** (`RegistrarPagamento`): marca a fatura paga, estende a licença até o fim do ciclo da competência (`fimDoCiclo`). Se a licença já estava vencida no calendário e não restam faturas abertas, **o novo período conta a partir de hoje** — quem quita o atraso ganha um ciclo inteiro. Se o ambiente estava suspenso por inadimplência, reativa.
6. **Inadimplência** (job diário): `MarcarLicencasVencidas` → `GerarFaturasDoMes` (uma fatura por competência vigente, sem duplicar) → `SuspenderPorInadimplencia` para grupos com fatura vencida há mais de `DIAS_ATRASO_SUSPENSAO` dias. O ambiente vira a página "ambiente suspenso" com o e-mail de suporte; os dados ficam intactos.
7. **Trocar plano** (`TrocarPlanoDaLicenca`): recalcula o valor, atualiza a assinatura no processador, muda o `LICENCA_USUARIOS_MAX` dos ambientes do grupo via provisionador (sem reprovisionar). O cliente pode pedir isso de dentro do CRM ("Meu plano" → `plano_solicitacoes`); o painel aprova ou recusa.
8. **Cancelar** (`CancelarLicenca`): desativa a assinatura (`enabledisable`), cancela faturas abertas, licença segue válida até o fim já pago e depois o job suspende.

## Modo simulado

`FIXPAY_SIMULADO=true` (padrão em `make dev`) não chama o processador: assinaturas ganham ids fictícios, o cartão `4111 1111 1111 1111` aprova e qualquer número terminado em `0002` recusa. Serve para demonstrar e para os testes de integração (`painel/internal/operacoes/operacoes_test.go`).

## O que o cliente vê

* Página `/painel/assinar/<codigo>`: cabeçalho CRM IA, resumo do plano, formulário do cartão, selo "pagamento seguro". Teste automatizado garante que a palavra do processador não aparece no HTML renderizado.
* E-mails: "Seu CRM IA está pronto", "Cadastre o cartão da sua assinatura", "Pagamento confirmado", "Fatura em atraso", todos com remetente CRM IA.
* Dentro do CRM: **Meu plano** (plano, usuários usados/contratados, próxima cobrança, final do cartão, link para atualizar cartão, botão "pedir upgrade").

## Atenção: dados de cartão passam pelo painel

Para o checkout ficar com a marca CRM IA, o número do cartão vai do navegador do cliente **para o painel** e dele para o gateway; ele não é gravado (só final e bandeira), não vai para o log nem para `cobranca_eventos`, e a página só é servida em HTTPS. Mesmo assim, isso coloca o painel no escopo PCI-DSS (SAQ A-EP, no mínimo). Antes de abrir para clientes reais, confirme com o processador se há um formulário hospedado ou tokenização no navegador que preserve a marca; se houver, troque `CadastrarCartao` para receber só o token e o painel sai do escopo.
