package web

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/painel/internal/modelo"
	"github.com/armando-couto/crm-ia/painel/internal/repo"
)

// -----------------------------------------------------------------------------
// Grupos
// -----------------------------------------------------------------------------

func (s *Servidor) listarGrupos(w http.ResponseWriter, r *http.Request) {
	d := s.base(r, "Grupos de CNPJ", "grupos")
	lista, err := s.repo.ListarGrupos(r.Context())
	if err != nil {
		d.Erro = err.Error()
	}
	d.V["Grupos"] = lista
	s.render(w, r, "grupos", d)
}

func (s *Servidor) verGrupo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, err := s.repo.GrupoPorID(ctx, idDaRota(r))
	if err != nil {
		s.erroHTTP(w, r, http.StatusNotFound, "Grupo não encontrado.")
		return
	}
	d := s.base(r, g.Nome, "grupos")
	d.V["Grupo"] = g
	clientes, _ := s.repo.ListarClientes(ctx, repo.FiltroClientes{GrupoID: g.ID})
	d.V["Clientes"] = clientes
	lics, _ := s.repo.LicencasDoGrupo(ctx, g.ID)
	d.V["Licencas"] = lics
	s.render(w, r, "grupo_ficha", d)
}

func (s *Servidor) formGrupo(w http.ResponseWriter, r *http.Request) {
	d := s.base(r, "Novo grupo", "grupos")
	g := modelo.Grupo{CobrancaUnificada: true, Ativo: true}
	if id := idDaRota(r); id > 0 {
		var err error
		if g, err = s.repo.GrupoPorID(r.Context(), id); err != nil {
			s.erroHTTP(w, r, http.StatusNotFound, "Grupo não encontrado.")
			return
		}
		d.Titulo = "Editar grupo"
	}
	d.V["Grupo"] = g
	s.render(w, r, "grupo_form", d)
}

func (s *Servidor) salvarGrupo(w http.ResponseWriter, r *http.Request) {
	g := modelo.Grupo{ID: idDaRota(r), Nome: campo(r, "nome"), CNPJResponsavel: somenteDigitos(campo(r, "cnpj_responsavel")),
		EmailFinanceiro: strings.ToLower(campo(r, "email_financeiro")), Telefone: campo(r, "telefone"),
		CobrancaUnificada: campo(r, "cobranca") != "por_cnpj", Observacoes: campo(r, "observacoes"), Ativo: true,
		CEP: somenteDigitos(campo(r, "cep")), Logradouro: campo(r, "logradouro"), Numero: campo(r, "numero"), Bairro: campo(r, "bairro"),
		Municipio: campo(r, "municipio"), UF: strings.ToUpper(campo(r, "uf"))}
	if g.ID > 0 {
		g.Ativo = campoBool(r, "ativo")
	}
	volta := "/grupos/novo"
	if g.ID > 0 {
		volta = fmt.Sprintf("/grupos/%d/editar", g.ID)
	}
	if g.Nome == "" {
		s.redirecionar(w, r, volta, "erro", "Informe o nome do grupo.")
		return
	}
	if g.CNPJResponsavel != "" && !cnpjValido(g.CNPJResponsavel) {
		s.redirecionar(w, r, volta, "erro", "CNPJ responsável inválido.")
		return
	}
	id, err := s.repo.SalvarGrupo(r.Context(), g)
	if err != nil {
		s.redirecionar(w, r, volta, "erro", "Não foi possível salvar: "+err.Error())
		return
	}
	s.auditar(r, "salvar", "grupo", &id, map[string]any{"nome": g.Nome})
	s.redirecionar(w, r, fmt.Sprintf("/grupos/%d", id), "ok", "Grupo salvo.")
}

// -----------------------------------------------------------------------------
// Clientes (CNPJ = ambiente)
// -----------------------------------------------------------------------------

var segmentos = []struct{ Codigo, Nome string }{
	{"vendas_b2b", "Vendas B2B"}, {"servicos_locais", "Serviços e atendimento local"}, {"imobiliaria", "Imobiliária"},
	{"varejo_ecommerce", "Varejo e e-commerce"}, {"agencia_consultoria", "Agência e consultoria"}, {"em_branco", "Outro / do zero"},
}

func (s *Servidor) listarClientes(w http.ResponseWriter, r *http.Request) {
	d := s.base(r, "Clientes", "clientes")
	f := repo.FiltroClientes{Status: r.URL.Query().Get("status"), Busca: r.URL.Query().Get("q")}
	lista, err := s.repo.ListarClientes(r.Context(), f)
	if err != nil {
		d.Erro = err.Error()
	}
	d.V["Clientes"], d.V["Filtro"] = lista, f
	d.V["Status"] = []modelo.StatusCliente{modelo.StatusPronto, modelo.StatusAtivo, modelo.StatusSuspenso, modelo.StatusErro, modelo.StatusCancelado}
	s.render(w, r, "clientes", d)
}

func (s *Servidor) formCliente(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d := s.base(r, "Novo cliente (CNPJ)", "clientes")
	c := modelo.Cliente{UsuariosContratados: 3, CorPrimaria: "#6d5df6", AutoAtualizar: true, ModeloCRM: "vendas_b2b"}
	if id := idDaRota(r); id > 0 {
		var err error
		if c, err = s.repo.ClientePorID(ctx, id); err != nil {
			s.erroHTTP(w, r, http.StatusNotFound, "Cliente não encontrado.")
			return
		}
		d.Titulo = "Editar cliente"
	} else if gid := r.URL.Query().Get("grupo"); gid != "" {
		fmt.Sscan(gid, &c.GrupoID)
	}
	grupos, _ := s.repo.ListarGrupos(ctx)
	planos, _ := s.repo.ListarPlanos(ctx, true)
	d.V["Cliente"], d.V["Grupos"], d.V["Planos"], d.V["Segmentos"] = c, grupos, planos, segmentos
	s.render(w, r, "cliente_form", d)
}

func (s *Servidor) salvarCliente(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := idDaRota(r)
	c := modelo.Cliente{ID: id, RazaoSocial: campo(r, "razao_social"), NomeFantasia: campo(r, "nome_fantasia"),
		CNPJ: somenteDigitos(campo(r, "cnpj")), Slug: strings.ToLower(campo(r, "slug")), Segmento: campo(r, "segmento"),
		Cidade: campo(r, "cidade"), UF: strings.ToUpper(campo(r, "uf")), Telefone: campo(r, "telefone"),
		CEP: somenteDigitos(campo(r, "cep")), Logradouro: campo(r, "logradouro"), Numero: campo(r, "numero"), Bairro: campo(r, "bairro"),
		AdminNome: campo(r, "admin_nome"), AdminEmail: strings.ToLower(campo(r, "admin_email")),
		PlanoID: campoInt64Ptr(r, "plano_id"), UsuariosContratados: campoInt(r, "usuarios_contratados", 3),
		CobrancaPropria: campoBool(r, "cobranca_propria"), ModeloCRM: campo(r, "modelo_crm"),
		AutoAtualizar: campoBool(r, "auto_atualizar"), CorPrimaria: campo(r, "cor_primaria"), LogoURL: campo(r, "logo_url")}
	if g := campoInt64Ptr(r, "grupo_id"); g != nil {
		c.GrupoID = *g
	}
	volta := "/clientes/novo"
	if id > 0 {
		volta = fmt.Sprintf("/clientes/%d/editar", id)
	}
	switch {
	case c.GrupoID == 0:
		s.redirecionar(w, r, volta, "erro", "Escolha o grupo do cliente (crie um grupo antes, se precisar).")
		return
	case c.RazaoSocial == "" || c.NomeFantasia == "":
		s.redirecionar(w, r, volta, "erro", "Informe razão social e nome fantasia.")
		return
	case !cnpjValido(c.CNPJ):
		s.redirecionar(w, r, volta, "erro", "CNPJ inválido.")
		return
	case c.AdminNome == "" || !strings.Contains(c.AdminEmail, "@"):
		s.redirecionar(w, r, volta, "erro", "Informe o administrador da empresa (nome e e-mail).")
		return
	}
	if c.CorPrimaria == "" {
		c.CorPrimaria = "#6d5df6"
	}
	if c.Slug == "" {
		c.Slug = gerarSlug(c.NomeFantasia)
	}
	if id == 0 {
		novoID, err := s.repo.CriarCliente(ctx, c)
		if err != nil {
			s.redirecionar(w, r, volta, "erro", "Não foi possível criar (CNPJ ou endereço já cadastrado?): "+err.Error())
			return
		}
		s.auditar(r, "criar", "cliente", &novoID, map[string]any{"cnpj": c.CNPJ, "slug": c.Slug})
		s.redirecionar(w, r, fmt.Sprintf("/clientes/%d", novoID), "ok", "Cliente criado. Provisione o ambiente e contrate a licença.")
		return
	}
	anterior, err := s.repo.ClientePorID(ctx, id)
	if err != nil {
		s.erroHTTP(w, r, http.StatusNotFound, "Cliente não encontrado.")
		return
	}
	c.Slug, c.CNPJ, c.Status = anterior.Slug, anterior.CNPJ, anterior.Status
	if err := s.repo.AtualizarCliente(ctx, c); err != nil {
		s.redirecionar(w, r, volta, "erro", "Não foi possível salvar: "+err.Error())
		return
	}
	s.auditar(r, "editar", "cliente", &id, nil)
	if anterior.Provisionado() && (anterior.CorPrimaria != c.CorPrimaria || anterior.LogoURL != c.LogoURL ||
		anterior.UsuariosContratados != c.UsuariosContratados || anterior.NomeFantasia != c.NomeFantasia) {
		if err := s.ops.AtualizarAmbiente(ctx, c); err != nil {
			s.redirecionar(w, r, fmt.Sprintf("/clientes/%d", id), "erro", "Salvo, mas o ambiente não foi atualizado: "+err.Error())
			return
		}
	}
	s.redirecionar(w, r, fmt.Sprintf("/clientes/%d", id), "ok", "Cliente atualizado.")
}

func (s *Servidor) verCliente(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, err := s.repo.ClientePorID(ctx, idDaRota(r))
	if err != nil {
		s.erroHTTP(w, r, http.StatusNotFound, "Cliente não encontrado.")
		return
	}
	d := s.base(r, c.NomeFantasia, "clientes")
	d.V["Cliente"] = c
	if lic, err := s.repo.LicencaDoCliente(ctx, c); err == nil {
		d.V["Licenca"] = lic
		if lic.CodigoCheckout != "" {
			d.V["URLCheckout"] = s.ops.URLCheckout(lic.CodigoCheckout)
		}
	}
	if sol, err := s.repo.UltimaSolicitacaoDoCliente(ctx, c.ID); err == nil {
		d.V["Solicitacao"] = sol
	}
	eventos, _ := s.repo.EventosDoCliente(ctx, c.ID, 20)
	d.V["Eventos"] = eventos
	versoes, _ := s.repo.ListarVersoes(ctx)
	d.V["Versoes"] = versoes
	if c.Provisionado() || c.Status == modelo.StatusErro || c.Status == modelo.StatusProvisionando {
		if st, err := s.prov.Status(ctx, c.Slug); err == nil {
			d.V["Ambiente"] = st
		} else {
			d.V["AmbienteErro"] = err.Error()
		}
	}
	if su, ok := s.tomarSenhaUnica(r, c.ID); ok {
		d.V["SenhaUnica"] = su
		w.Header().Set("Cache-Control", "no-store")
	}
	s.render(w, r, "cliente_ficha", d)
}

func (s *Servidor) provisionarCliente(w http.ResponseWriter, r *http.Request) {
	id := idDaRota(r)
	res, err := s.ops.Provisionar(r.Context(), id, campo(r, "versao"))
	if err != nil {
		s.redirecionar(w, r, fmt.Sprintf("/clientes/%d", id), "erro", "Provisionamento falhou: "+err.Error())
		return
	}
	s.auditar(r, "provisionar", "cliente", &id, map[string]any{"versao": res.Versao})
	msg := "Ambiente no ar em " + res.URL
	if res.SenhaAdmin != "" {
		s.guardarSenhaUnica(r, id, "Senha inicial do administrador", "", res.SenhaAdmin)
		msg += ". Anote a senha inicial do administrador: ela não será exibida de novo."
	}
	s.redirecionar(w, r, fmt.Sprintf("/clientes/%d", id), "ok", msg)
}

func (s *Servidor) atualizarVersaoCliente(w http.ResponseWriter, r *http.Request) {
	id := idDaRota(r)
	versao := campo(r, "versao")
	if versao == "" {
		s.redirecionar(w, r, fmt.Sprintf("/clientes/%d", id), "erro", "Escolha a versão.")
		return
	}
	if err := s.ops.AtualizarVersao(r.Context(), id, versao); err != nil {
		s.redirecionar(w, r, fmt.Sprintf("/clientes/%d", id), "erro", "Atualização falhou: "+err.Error())
		return
	}
	s.auditar(r, "atualizar", "cliente", &id, map[string]any{"versao": versao})
	s.redirecionar(w, r, fmt.Sprintf("/clientes/%d", id), "ok", "Ambiente atualizado para a versão "+versao+".")
}

func (s *Servidor) suspenderCliente(w http.ResponseWriter, r *http.Request) {
	id := idDaRota(r)
	if err := s.ops.Suspender(r.Context(), id, "suspensão manual por "+sessaoDe(r).Email); err != nil {
		s.redirecionar(w, r, fmt.Sprintf("/clientes/%d", id), "erro", err.Error())
		return
	}
	s.auditar(r, "suspender", "cliente", &id, nil)
	s.redirecionar(w, r, fmt.Sprintf("/clientes/%d", id), "ok", "Acesso suspenso. Os dados foram preservados.")
}

func (s *Servidor) reativarCliente(w http.ResponseWriter, r *http.Request) {
	id := idDaRota(r)
	if err := s.ops.Reativar(r.Context(), id); err != nil {
		s.redirecionar(w, r, fmt.Sprintf("/clientes/%d", id), "erro", err.Error())
		return
	}
	s.auditar(r, "reativar", "cliente", &id, nil)
	s.redirecionar(w, r, fmt.Sprintf("/clientes/%d", id), "ok", "Acesso reativado.")
}

func (s *Servidor) backupCliente(w http.ResponseWriter, r *http.Request) {
	id := idDaRota(r)
	arquivo, err := s.ops.Backup(r.Context(), id)
	if err != nil {
		s.redirecionar(w, r, fmt.Sprintf("/clientes/%d", id), "erro", "Backup falhou: "+err.Error())
		return
	}
	s.auditar(r, "backup", "cliente", &id, map[string]any{"arquivo": arquivo})
	s.redirecionar(w, r, fmt.Sprintf("/clientes/%d", id), "ok", "Backup gerado: "+arquivo)
}

func (s *Servidor) removerCliente(w http.ResponseWriter, r *http.Request) {
	id := idDaRota(r)
	c, err := s.repo.ClientePorID(r.Context(), id)
	if err != nil {
		s.erroHTTP(w, r, http.StatusNotFound, "Cliente não encontrado.")
		return
	}
	if campo(r, "confirmar") != c.Slug {
		s.redirecionar(w, r, fmt.Sprintf("/clientes/%d", id), "erro", "Para remover, digite o endereço (slug) do cliente exatamente como aparece.")
		return
	}
	if err := s.ops.Remover(r.Context(), id); err != nil {
		s.redirecionar(w, r, fmt.Sprintf("/clientes/%d", id), "erro", "Remoção falhou: "+err.Error())
		return
	}
	s.auditar(r, "remover", "cliente", &id, map[string]any{"slug": c.Slug})
	s.redirecionar(w, r, "/clientes", "ok", "Ambiente removido (backup guardado) e cliente cancelado.")
}

// redefinirSenhaAdmin pede ao ambiente uma senha provisória para o administrador.
func (s *Servidor) redefinirSenhaAdmin(w http.ResponseWriter, r *http.Request) {
	id := idDaRota(r)
	email, senha, err := s.redefinirSenhaNoAmbiente(r, id)
	if err != nil {
		s.redirecionar(w, r, fmt.Sprintf("/clientes/%d", id), "erro", err.Error())
		return
	}
	s.auditar(r, "senha_admin", "cliente", &id, map[string]any{"email": email})
	s.guardarSenhaUnica(r, id, "Senha provisória do administrador", email, senha)
	s.redirecionar(w, r, fmt.Sprintf("/clientes/%d", id), "ok", "Senha provisória gerada. Ela aparece uma única vez abaixo.")
}

// -----------------------------------------------------------------------------
// Ambientes: tudo o que está no ar, lido do provisionador
// -----------------------------------------------------------------------------

type linhaAmbiente struct {
	Slug, Nome, URL, StatusReal, StatusPainel, Versao string
	ClienteID                                         int64
	Containers                                        int
	Desatualizado, AutoAtualizar                      bool
}

func (s *Servidor) listarAmbientes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d := s.base(r, "Ambientes", "ambientes")
	ambientes, err := s.prov.Listar(ctx)
	if err != nil {
		d.Erro = "Não foi possível consultar o provisionador: " + err.Error()
	}
	clientes, _ := s.repo.ListarClientes(ctx, repo.FiltroClientes{})
	porSlug := map[string]modelo.Cliente{}
	for _, c := range clientes {
		porSlug[c.Slug] = c
	}
	padrao, _ := s.repo.VersaoPadrao(ctx)
	var linhas []linhaAmbiente
	for _, a := range ambientes {
		l := linhaAmbiente{Slug: a.Slug, URL: a.URL, StatusReal: a.Status, Versao: a.Versao, Containers: len(a.Containers)}
		if c, ok := porSlug[a.Slug]; ok {
			l.Nome, l.ClienteID, l.StatusPainel, l.AutoAtualizar = c.NomeFantasia, c.ID, string(c.Status), c.AutoAtualizar
		}
		l.Desatualizado = padrao.Tag != "" && a.Versao != "" && a.Versao != padrao.Tag
		linhas = append(linhas, l)
	}
	d.V["Linhas"], d.V["Padrao"] = linhas, padrao
	s.render(w, r, "ambientes", d)
}

// hoje é usada pelos templates via função; mantida aqui para o pacote compilar
// mesmo sem uso direto em handlers.
var _ = time.Now
