package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/painel/internal/email"
	"github.com/armando-couto/crm-ia/painel/internal/modelo"
	"github.com/armando-couto/crm-ia/painel/internal/operacoes"
	"github.com/armando-couto/crm-ia/painel/internal/repo"
)

// -----------------------------------------------------------------------------
// Licenças
// -----------------------------------------------------------------------------

func (s *Servidor) listarLicencas(w http.ResponseWriter, r *http.Request) {
	d := s.base(r, "Licenças", "licencas")
	status := r.URL.Query().Get("status")
	lista, err := s.repo.ListarLicencas(r.Context(), status)
	if err != nil {
		d.Erro = err.Error()
	}
	d.V["Licencas"], d.V["Status"] = lista, status
	d.V["URLCheckout"] = s.ops.URLCheckout
	s.render(w, r, "licencas", d)
}

func (s *Servidor) formLicenca(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d := s.base(r, "Nova licença", "licencas")
	grupos, _ := s.repo.ListarGrupos(ctx)
	clientes, _ := s.repo.ListarClientes(ctx, repo.FiltroClientes{})
	planos, _ := s.repo.ListarPlanos(ctx, true)
	d.V["Grupos"], d.V["Clientes"], d.V["Planos"] = grupos, clientes, planos
	d.V["GrupoID"], d.V["ClienteID"] = r.URL.Query().Get("grupo"), r.URL.Query().Get("cliente")
	d.V["CobrancaConfigurada"] = s.ops.Sign().Configurado()
	d.V["MotivoSemCobranca"] = s.ops.Sign().MotivoNaoConfigurado()
	d.V["Simulado"] = s.ops.Sign().Simulado()
	s.render(w, r, "licenca_form", d)
}

func (s *Servidor) criarLicenca(w http.ResponseWriter, r *http.Request) {
	inicio, err := time.ParseInLocation("2006-01-02", campo(r, "inicio"), time.Local)
	if err != nil {
		inicio = time.Now()
	}
	p := operacoes.PedidoLicenca{ClienteID: campoInt64Ptr(r, "cliente_id"), UsuariosMax: campoInt(r, "usuarios_max", 0),
		Periodicidade: campo(r, "periodicidade"), Inicio: inicio, Unidades: campoInt(r, "unidades", 1), Observacoes: campo(r, "observacoes"),
		Cobranca: campo(r, "cobranca")}
	if g := campoInt64Ptr(r, "grupo_id"); g != nil {
		p.GrupoID = *g
	}
	if pl := campoInt64Ptr(r, "plano_id"); pl != nil {
		p.PlanoID = *pl
	}
	if p.GrupoID == 0 || p.PlanoID == 0 {
		s.redirecionar(w, r, "/licencas/nova", "erro", "Escolha o grupo e o plano.")
		return
	}
	if p.ClienteID != nil {
		c, err := s.repo.ClientePorID(r.Context(), *p.ClienteID)
		if err != nil || c.GrupoID != p.GrupoID {
			s.redirecionar(w, r, "/licencas/nova", "erro", "O CNPJ escolhido não pertence ao grupo.")
			return
		}
		p.Unidades = 1
	}
	res, err := s.ops.ContratarLicenca(r.Context(), p)
	if err != nil {
		s.redirecionar(w, r, "/licencas/nova", "erro", "Não foi possível contratar: "+err.Error())
		return
	}
	s.auditar(r, "criar", "licenca", &res.ID, map[string]any{"grupo": p.GrupoID, "plano": p.PlanoID, "cobranca": p.Cobranca})
	msg := "Licença criada."
	if res.URLCheckout != "" {
		msg += " Envie ao cliente o link de cadastro do cartão (na lista abaixo, botão “Enviar link”) — a cobrança mensal começa quando ele cadastrar."
	}
	s.redirecionar(w, r, "/licencas", "ok", msg)
}

func (s *Servidor) renovarLicenca(w http.ResponseWriter, r *http.Request) {
	id := idDaRota(r)
	l, err := s.repo.LicencaPorID(r.Context(), id)
	if err != nil {
		s.redirecionar(w, r, "/licencas", "erro", erroAmigavel(err))
		return
	}
	base := l.Fim
	if base.Before(time.Now()) {
		base = time.Now()
	}
	novoFim := base.AddDate(0, 1, 0)
	if l.Periodicidade == "anual" {
		novoFim = base.AddDate(1, 0, 0)
	}
	if err := s.repo.RenovarLicenca(r.Context(), id, novoFim); err != nil {
		s.redirecionar(w, r, "/licencas", "erro", err.Error())
		return
	}
	s.auditar(r, "renovar", "licenca", &id, map[string]any{"fim": novoFim.Format("2006-01-02")})
	s.redirecionar(w, r, "/licencas", "ok", "Licença renovada até "+novoFim.Format("02/01/2006")+".")
}

func (s *Servidor) cancelarLicenca(w http.ResponseWriter, r *http.Request) {
	id := idDaRota(r)
	if err := s.ops.CancelarLicenca(r.Context(), id); err != nil {
		s.redirecionar(w, r, "/licencas", "erro", err.Error())
		return
	}
	s.auditar(r, "cancelar", "licenca", &id, nil)
	s.redirecionar(w, r, "/licencas", "ok", "Licença e recorrência canceladas.")
}

// enviarLinkCartao manda ao administrador do cliente (ou ao financeiro do
// grupo) o e-mail com o link da página de cadastro do cartão.
func (s *Servidor) enviarLinkCartao(w http.ResponseWriter, r *http.Request) {
	id := idDaRota(r)
	ctx := r.Context()
	l, err := s.repo.LicencaPorID(ctx, id)
	if err != nil || l.CodigoCheckout == "" {
		s.redirecionar(w, r, "/licencas", "erro", "Esta licença não tem cobrança recorrente pendente.")
		return
	}
	destino, nome := "", ""
	if l.ClienteID != nil {
		if c, err := s.repo.ClientePorID(ctx, *l.ClienteID); err == nil {
			destino, nome = c.AdminEmail, c.AdminNome
		}
	} else if g, err := s.repo.GrupoPorID(ctx, l.GrupoID); err == nil {
		destino, nome = g.EmailFinanceiro, g.Nome
	}
	if e := campo(r, "email"); e != "" {
		destino = e
	}
	if !strings.Contains(destino, "@") {
		s.redirecionar(w, r, "/licencas", "erro", "Informe um e-mail de destino.")
		return
	}
	link := s.ops.URLCheckout(l.CodigoCheckout)
	corpo := fmt.Sprintf(`<p>Olá, %s.</p><p>Sua assinatura do plano <strong>%s</strong> (%s por mês) está pronta. Para ativar a cobrança mensal automática no cartão de crédito, cadastre o cartão pelo link abaixo — leva um minuto e a página é segura.</p>
<p><a href="%s" style="display:inline-block;background:#6d5df6;color:#fff;padding:12px 22px;border-radius:8px;text-decoration:none">Cadastrar cartão</a></p>
<p>Ou copie o endereço: <br><code>%s</code></p><p>Dúvidas? Responda este e-mail ou fale com %s.</p>`,
		nome, l.PlanoNome, modelo.FormatarBRL(l.ValorCentavos), link, link, s.cfg.EmailSuporte)
	err = s.correio.Enviar(ctx, email.Mensagem{Para: destino, ParaNome: nome, Assunto: "Ative sua assinatura do " + s.cfg.NomeProduto,
		HTML: email.Layout(s.cfg.NomeProduto, "#6d5df6", "Cadastre o cartão da sua assinatura", corpo)})
	if err != nil {
		s.redirecionar(w, r, "/licencas", "erro", "O e-mail não saiu: "+err.Error()+". O link é "+link)
		return
	}
	s.auditar(r, "enviar_link", "licenca", &id, map[string]any{"para": destino})
	s.redirecionar(w, r, "/licencas", "ok", "Link de cadastro do cartão enviado para "+destino+".")
}

// -----------------------------------------------------------------------------
// Faturas
// -----------------------------------------------------------------------------

func (s *Servidor) listarFaturas(w http.ResponseWriter, r *http.Request) {
	d := s.base(r, "Faturas", "faturas")
	status := r.URL.Query().Get("status")
	lista, err := s.repo.ListarFaturas(r.Context(), status)
	if err != nil {
		d.Erro = err.Error()
	}
	var pendente, vencido int64
	hoje := time.Now()
	for _, f := range lista {
		switch {
		case f.Status == "vencida" || (f.Status == "pendente" && f.Vencimento.Before(hoje)):
			vencido += f.ValorCentavos
		case f.Status == "pendente":
			pendente += f.ValorCentavos
		}
	}
	d.V["Faturas"], d.V["Status"], d.V["TotalPendente"], d.V["TotalVencido"] = lista, status, pendente, vencido
	d.V["Simulado"] = s.ops.Sign().Simulado()
	s.render(w, r, "faturas", d)
}

func (s *Servidor) conferirPagamentos(w http.ResponseWriter, r *http.Request) {
	n, err := s.ops.ConferirPagamentos(r.Context())
	if err != nil {
		s.redirecionar(w, r, "/faturas", "erro", "Conferência falhou: "+err.Error())
		return
	}
	s.auditar(r, "conferir", "faturas", nil, map[string]any{"baixadas": n})
	s.redirecionar(w, r, "/faturas", "ok", fmt.Sprintf("Conferência concluída: %d fatura(s) baixada(s).", n))
}

func (s *Servidor) baixarFatura(w http.ResponseWriter, r *http.Request) {
	id := idDaRota(r)
	res, err := s.ops.RegistrarPagamento(r.Context(), id, "manual", "manual:"+sessaoDe(r).Email)
	if err != nil {
		s.redirecionar(w, r, "/faturas", "erro", err.Error())
		return
	}
	if !res.Baixada {
		s.redirecionar(w, r, "/faturas", "aviso", "Esta fatura já estava fechada.")
		return
	}
	s.auditar(r, "baixar", "fatura", &id, map[string]any{"reativados": res.Reativados})
	msg := "Fatura baixada e licença renovada."
	if len(res.Reativados) > 0 {
		msg += " Ambientes reativados: " + strings.Join(res.Reativados, ", ") + "."
	}
	s.redirecionar(w, r, "/faturas", "ok", msg)
}

func (s *Servidor) cancelarFatura(w http.ResponseWriter, r *http.Request) {
	id := idDaRota(r)
	if err := s.repo.CancelarFatura(r.Context(), id); err != nil {
		s.redirecionar(w, r, "/faturas", "erro", err.Error())
		return
	}
	s.auditar(r, "cancelar", "fatura", &id, nil)
	s.redirecionar(w, r, "/faturas", "ok", "Fatura cancelada.")
}

// -----------------------------------------------------------------------------
// Planos e configurações
// -----------------------------------------------------------------------------

func (s *Servidor) listarPlanos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d := s.base(r, "Planos e preços", "planos")
	planos, err := s.repo.ListarPlanos(ctx, false)
	if err != nil {
		d.Erro = err.Error()
	}
	confs, _ := s.repo.Configuracoes(ctx)
	d.V["Planos"], d.V["Configuracoes"] = planos, confs
	s.render(w, r, "planos", d)
}

func (s *Servidor) formPlano(w http.ResponseWriter, r *http.Request) {
	d := s.base(r, "Novo plano", "planos")
	p := modelo.Plano{UsuariosMin: 1, UsuariosMax: 5, Ativo: true}
	if id := idDaRota(r); id > 0 {
		var err error
		if p, err = s.repo.PlanoPorID(r.Context(), id); err != nil {
			s.erroHTTP(w, r, http.StatusNotFound, "Plano não encontrado.")
			return
		}
		d.Titulo = "Editar plano"
	}
	d.V["Plano"] = p
	s.render(w, r, "plano_form", d)
}

func (s *Servidor) salvarPlano(w http.ResponseWriter, r *http.Request) {
	p := modelo.Plano{ID: idDaRota(r), Codigo: strings.ToLower(campo(r, "codigo")), Nome: campo(r, "nome"), Descricao: campo(r, "descricao"),
		UsuariosMin: campoInt(r, "usuarios_min", 1), UsuariosMax: campoInt(r, "usuarios_max", 1),
		PrecoMensalCentavos: campoCentavos(r, "preco_mensal"), PrecoAnualCentavos: campoCentavos(r, "preco_anual"),
		Destaque: campoBool(r, "destaque"), Ordem: campoInt(r, "ordem", 0), Ativo: campoBool(r, "ativo")}
	for _, linha := range strings.Split(campo(r, "recursos"), "\n") {
		if l := strings.TrimSpace(linha); l != "" {
			p.Recursos = append(p.Recursos, l)
		}
	}
	if p.Codigo == "" || p.Nome == "" || p.UsuariosMax < p.UsuariosMin {
		s.redirecionar(w, r, "/planos", "erro", "Informe código, nome e uma faixa de usuários válida.")
		return
	}
	id, err := s.repo.SalvarPlano(r.Context(), p)
	if err != nil {
		s.redirecionar(w, r, "/planos", "erro", "Não foi possível salvar: "+err.Error())
		return
	}
	s.auditar(r, "salvar", "plano", &id, map[string]any{"codigo": p.Codigo, "mensal": p.PrecoMensalCentavos})
	s.redirecionar(w, r, "/planos", "ok", "Plano salvo. A landing já reflete o novo preço; licenças existentes mantêm o valor contratado.")
}

func (s *Servidor) salvarConfiguracoes(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	for chave, valores := range r.PostForm {
		if !strings.HasPrefix(chave, "cfg.") || len(valores) == 0 {
			continue
		}
		if err := s.repo.SalvarConfiguracao(r.Context(), strings.TrimPrefix(chave, "cfg."), strings.TrimSpace(valores[0])); err != nil {
			s.redirecionar(w, r, "/planos", "erro", err.Error())
			return
		}
	}
	s.auditar(r, "salvar", "configuracoes", nil, nil)
	s.redirecionar(w, r, "/planos", "ok", "Configurações salvas.")
}

// -----------------------------------------------------------------------------
// Solicitações de plano
// -----------------------------------------------------------------------------

func (s *Servidor) listarSolicitacoes(w http.ResponseWriter, r *http.Request) {
	d := s.base(r, "Solicitações de plano", "solicitacoes")
	status := r.URL.Query().Get("status")
	lista, err := s.repo.ListarSolicitacoes(r.Context(), status)
	if err != nil {
		d.Erro = err.Error()
	}
	d.V["Solicitacoes"], d.V["Status"] = lista, status
	s.render(w, r, "solicitacoes", d)
}

func (s *Servidor) aprovarSolicitacao(w http.ResponseWriter, r *http.Request) {
	id := idDaRota(r)
	resposta, err := s.ops.AprovarSolicitacao(r.Context(), id, sessaoDe(r).Email)
	if err != nil {
		s.redirecionar(w, r, "/solicitacoes", "erro", "Não foi possível aprovar: "+err.Error())
		return
	}
	s.auditar(r, "aprovar", "solicitacao", &id, map[string]any{"resposta": resposta})
	s.redirecionar(w, r, "/solicitacoes", "ok", "Aprovada. "+resposta)
}

func (s *Servidor) recusarSolicitacao(w http.ResponseWriter, r *http.Request) {
	id := idDaRota(r)
	if err := s.ops.RecusarSolicitacao(r.Context(), id, campo(r, "resposta"), sessaoDe(r).Email); err != nil {
		s.redirecionar(w, r, "/solicitacoes", "erro", err.Error())
		return
	}
	s.auditar(r, "recusar", "solicitacao", &id, nil)
	s.redirecionar(w, r, "/solicitacoes", "ok", "Solicitação recusada — a resposta aparece no CRM da empresa.")
}

// -----------------------------------------------------------------------------
// Versões
// -----------------------------------------------------------------------------

func (s *Servidor) listarVersoes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d := s.base(r, "Versões do aplicativo", "versoes")
	lista, err := s.repo.ListarVersoes(ctx)
	if err != nil {
		d.Erro = err.Error()
	}
	d.V["Versoes"] = lista
	if tags, err := s.prov.VersoesDisponiveis(ctx); err == nil {
		d.V["TagsNoHost"] = tags
	} else {
		d.Aviso = "Não foi possível listar as imagens no host: " + err.Error()
	}
	clientes, _ := s.repo.ListarClientes(ctx, repo.FiltroClientes{Status: "ativo"})
	d.V["Clientes"] = clientes
	s.render(w, r, "versoes", d)
}

func (s *Servidor) criarVersao(w http.ResponseWriter, r *http.Request) {
	v := modelo.Versao{Tag: campo(r, "tag"), Descricao: campo(r, "descricao"), Changelog: campo(r, "changelog"), Estavel: campoBool(r, "estavel")}
	if v.Tag == "" || strings.ContainsAny(v.Tag, " /:") {
		s.redirecionar(w, r, "/versoes", "erro", "Informe uma tag válida (ex.: 1.2.0).")
		return
	}
	id, err := s.repo.CriarVersao(r.Context(), v)
	if err != nil {
		s.redirecionar(w, r, "/versoes", "erro", "Não foi possível cadastrar (tag repetida?).")
		return
	}
	s.auditar(r, "publicar", "versao", &id, map[string]any{"tag": v.Tag})
	s.redirecionar(w, r, "/versoes", "ok", "Versão "+v.Tag+" cadastrada. Defina-a como padrão para clientes novos e para quem acompanha a padrão.")
}

func (s *Servidor) definirVersaoPadrao(w http.ResponseWriter, r *http.Request) {
	id := idDaRota(r)
	if err := s.repo.DefinirVersaoPadrao(r.Context(), id); err != nil {
		s.redirecionar(w, r, "/versoes", "erro", err.Error())
		return
	}
	s.auditar(r, "definir_padrao", "versao", &id, nil)
	s.redirecionar(w, r, "/versoes", "ok", "Versão padrão definida. Quem acompanha a padrão é movido na próxima rotina (ou use “aplicar agora”).")
}

func (s *Servidor) removerVersao(w http.ResponseWriter, r *http.Request) {
	id := idDaRota(r)
	if err := s.repo.RemoverVersao(r.Context(), id); err != nil {
		s.redirecionar(w, r, "/versoes", "erro", err.Error())
		return
	}
	s.redirecionar(w, r, "/versoes", "ok", "Versão removida do catálogo (a imagem continua no host).")
}

func (s *Servidor) aplicarVersaoEmMassa(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	tag := campo(r, "tag")
	ids := r.Form["clientes"]
	if tag == "" || len(ids) == 0 {
		s.redirecionar(w, r, "/versoes", "erro", "Escolha a versão e ao menos um cliente.")
		return
	}
	ok, falhas := 0, []string{}
	for _, sid := range ids {
		var id int64
		fmt.Sscan(sid, &id)
		if err := s.ops.AtualizarVersao(r.Context(), id, tag); err != nil {
			falhas = append(falhas, sid+": "+err.Error())
		} else {
			ok++
		}
	}
	s.auditar(r, "aplicar_versao", "versao", nil, map[string]any{"tag": tag, "ok": ok, "falhas": len(falhas)})
	msg := fmt.Sprintf("%d ambiente(s) atualizados para %s.", ok, tag)
	if len(falhas) > 0 {
		s.redirecionar(w, r, "/versoes", "erro", msg+" Falhas: "+strings.Join(falhas, "; "))
		return
	}
	s.redirecionar(w, r, "/versoes", "ok", msg)
}

// -----------------------------------------------------------------------------
// Leads
// -----------------------------------------------------------------------------

func (s *Servidor) listarLeads(w http.ResponseWriter, r *http.Request) {
	d := s.base(r, "Leads da landing", "leads")
	lista, err := s.repo.ListarLeads(r.Context())
	if err != nil {
		d.Erro = err.Error()
	}
	d.V["Leads"] = lista
	s.render(w, r, "leads", d)
}

func (s *Servidor) atualizarLead(w http.ResponseWriter, r *http.Request) {
	id := idDaRota(r)
	status := campo(r, "status")
	if status != "novo" && status != "contato" && status != "convertido" && status != "perdido" {
		s.redirecionar(w, r, "/leads", "erro", "Status inválido.")
		return
	}
	if err := s.repo.AtualizarStatusLead(r.Context(), id, status); err != nil {
		s.redirecionar(w, r, "/leads", "erro", err.Error())
		return
	}
	s.redirecionar(w, r, "/leads", "ok", "Lead atualizado.")
}

// -----------------------------------------------------------------------------
// Senha do administrador do ambiente (rota interna do app do cliente)
// -----------------------------------------------------------------------------

func (s *Servidor) redefinirSenhaNoAmbiente(r *http.Request, clienteID int64) (string, string, error) {
	ctx, cancelar := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
	defer cancelar()
	c, err := s.repo.ClientePorID(ctx, clienteID)
	if err != nil {
		return "", "", err
	}
	if c.Status != modelo.StatusAtivo {
		return "", "", fmt.Errorf("o ambiente não está no ar (%s)", c.Status.Rotulo())
	}
	token, err := s.prov.TokenInterno(ctx, c.Slug)
	if err != nil {
		return "", "", fmt.Errorf("sem acesso ao ambiente: %w", err)
	}
	corpo, _ := json.Marshal(map[string]string{"email": c.AdminEmail})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf(s.cfg.TenantAPIMolde, c.Slug)+"/interno/admin/senha", bytes.NewReader(corpo))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", "", errors.New("o ambiente da empresa não respondeu")
	}
	defer resp.Body.Close()
	var out struct {
		Email string `json:"email"`
		Senha string `json:"senha"`
		Erro  string `json:"erro"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out)
	switch {
	case resp.StatusCode == http.StatusOK && out.Senha != "":
	case out.Erro != "":
		return "", "", errors.New(out.Erro)
	default:
		return "", "", fmt.Errorf("o ambiente respondeu %d", resp.StatusCode)
	}
	s.repo.RegistrarEvento(ctx, c.ID, "senha", "senha provisória do administrador ("+out.Email+") gerada por "+sessaoDe(r).Email)
	return out.Email, out.Senha, nil
}
