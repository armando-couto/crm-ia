package controllers

import "github.com/kataras/iris/v12"

// Fila de ligações: próximo lead com trava, resultado da ligação em um clique,
// resumo do dia e disponibilidade da agenda. Contrato: docs/PROSPECCAO.md
// seção 8.2, com as correções de docs/PROSPECCAO-ADENDO.md.
//
// Rotas já registradas em routes/routes.go; cada handler abaixo responde 501
// até ser implementado (naoImplementado, em prospeccao_stub.go).

// ResultadoLead: POST /leads/{id}/resultado (ResultadoRequest) → ResultadoResposta.
func ResultadoLead(ctx iris.Context) { naoImplementado(ctx) }

// LiberarLead: POST /leads/{id}/liberar → 204 (solta a trava da fila; sem efeito para quem não a detém).
func LiberarLead(ctx iris.Context) { naoImplementado(ctx) }

// FilaResumo: GET /leads/fila/resumo → ResumoFila.
func FilaResumo(ctx iris.Context) { naoImplementado(ctx) }

// FilaProximo: POST /leads/fila/proximo { lista_id?, pular_ids? } → { lead, motivo_vazio, janela }.
func FilaProximo(ctx iris.Context) { naoImplementado(ctx) }

// AgendaDisponibilidade: GET /agenda/disponibilidade?user_id&dias → horários livres e ocupados por dia.
func AgendaDisponibilidade(ctx iris.Context) { naoImplementado(ctx) }
