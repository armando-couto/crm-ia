package web

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/armando-couto/crm-ia/painel/internal/auth"
	"github.com/armando-couto/crm-ia/painel/internal/modelo"
	"github.com/armando-couto/crm-ia/painel/internal/operacoes"
	"github.com/armando-couto/crm-ia/painel/internal/repo"
	"github.com/armando-couto/crm-ia/painel/internal/sign"
)

// -----------------------------------------------------------------------------
// Login / logout
// -----------------------------------------------------------------------------

func (s *Servidor) telaLogin(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "login", s.base(r, "Entrar", "login"))
}

func (s *Servidor) processarLogin(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(campo(r, "email"))
	senha := r.PostFormValue("senha")
	d := s.base(r, "Entrar", "login")
	d.V["Email"] = email
	u, err := s.repo.UsuarioPorEmail(r.Context(), email)
	if err != nil || !u.Ativo || !auth.SenhaConfere(u.SenhaHash, senha) {
		if err != nil {
			_, _ = auth.HashSenha("tempo-constante") // mesmo tempo para usuário inexistente
		}
		d.Erro = "E-mail ou senha inválidos."
		w.WriteHeader(http.StatusUnauthorized)
		s.render(w, r, "login", d)
		return
	}
	if err := s.sessoes.Iniciar(w, u); err != nil {
		d.Erro = "Não foi possível iniciar a sessão."
		s.render(w, r, "login", d)
		return
	}
	_ = s.repo.RegistrarLogin(r.Context(), u.ID)
	s.repo.Auditar(r.Context(), &u.ID, u.Email, "login", "usuario", &u.ID, nil, ipDe(r))
	http.Redirect(w, r, s.url("/"), http.StatusSeeOther)
}

func (s *Servidor) logout(w http.ResponseWriter, r *http.Request) {
	s.sessoes.Encerrar(w)
	http.Redirect(w, r, s.url("/login"), http.StatusSeeOther)
}

// -----------------------------------------------------------------------------
// Visão geral
// -----------------------------------------------------------------------------

func (s *Servidor) painel(w http.ResponseWriter, r *http.Request) {
	d := s.base(r, "Visão geral", "painel")
	ctx := r.Context()
	res, err := s.repo.Resumo(ctx, time.Now(), 15)
	if err != nil {
		slog.Error("resumo", "erro", err)
		d.Erro = "Não foi possível calcular os indicadores agora: " + err.Error()
	}
	d.V["Resumo"] = res
	lics, _ := s.repo.ListarLicencas(ctx, "ativa")
	var vencendo, semCartao []modelo.Licenca
	for _, l := range lics {
		if l.DiasParaVencer(time.Now()) <= 15 {
			vencendo = append(vencendo, l)
		}
		if l.Recorrente() && l.PagamentoStatus == modelo.PagamentoAguardandoCartao {
			semCartao = append(semCartao, l)
		}
	}
	d.V["Vencendo"], d.V["SemCartao"] = vencendo, semCartao
	aguardando, _ := s.repo.ListarClientes(ctx, repo.FiltroClientes{Status: "pronto"})
	d.V["Aguardando"] = aguardando
	leads, _ := s.repo.ListarLeads(ctx)
	if len(leads) > 6 {
		leads = leads[:6]
	}
	d.V["Leads"] = leads
	d.V["Simulado"] = s.ops.Sign().Simulado()
	d.V["CobrancaConfigurada"] = s.ops.Sign().Configurado()
	d.V["MotivoSemCobranca"] = s.ops.Sign().MotivoNaoConfigurado()
	s.render(w, r, "painel", d)
}

// -----------------------------------------------------------------------------
// API pública: planos (landing) e leads
// -----------------------------------------------------------------------------

type planoPublico struct {
	Codigo      string   `json:"codigo"`
	Nome        string   `json:"nome"`
	Descricao   string   `json:"descricao"`
	UsuariosMin int      `json:"usuarios_min"`
	UsuariosMax int      `json:"usuarios_max"`
	Faixa       string   `json:"faixa"`
	PrecoMensal int64    `json:"preco_mensal_centavos"`
	PrecoAnual  int64    `json:"preco_anual_centavos"`
	SobConsulta bool     `json:"sob_consulta"`
	Destaque    bool     `json:"destaque"`
	Recursos    []string `json:"recursos"`
}

func (s *Servidor) apiPlanos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	planos, err := s.repo.ListarPlanos(ctx, true)
	if err != nil {
		s.json(w, http.StatusInternalServerError, map[string]string{"erro": "indisponível"})
		return
	}
	saida := []planoPublico{}
	for _, p := range planos {
		saida = append(saida, planoPublico{Codigo: p.Codigo, Nome: p.Nome, Descricao: p.Descricao, UsuariosMin: p.UsuariosMin, UsuariosMax: p.UsuariosMax,
			Faixa: p.Faixa(), PrecoMensal: p.PrecoMensalCentavos, PrecoAnual: p.PrecoAnualCentavos, SobConsulta: p.SobConsulta(), Destaque: p.Destaque, Recursos: p.Recursos})
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=300")
	s.json(w, http.StatusOK, map[string]any{
		"planos": saida,
		"textos": map[string]string{
			"titulo":     s.repo.Configuracao(ctx, "landing.titulo", ""),
			"subtitulo":  s.repo.Configuracao(ctx, "landing.subtitulo", ""),
			"dias_teste": s.repo.Configuracao(ctx, "landing.dias_teste", "14"),
			"whatsapp":   s.repo.Configuracao(ctx, "landing.whatsapp", ""),
		},
	})
}

func (s *Servidor) apiCriarLead(w http.ResponseWriter, r *http.Request) {
	var l modelo.Lead
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var corpo struct {
			Nome, Email, Telefone, Empresa, Segmento, Cidade, Plano, Mensagem string
			Usuarios                                                          int
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&corpo); err != nil {
			s.json(w, http.StatusBadRequest, map[string]string{"erro": "corpo inválido"})
			return
		}
		l = modelo.Lead{Nome: corpo.Nome, Email: corpo.Email, Telefone: corpo.Telefone, Empresa: corpo.Empresa, Segmento: corpo.Segmento,
			Cidade: corpo.Cidade, Usuarios: corpo.Usuarios, PlanoInteresse: corpo.Plano, Mensagem: corpo.Mensagem}
	} else {
		_ = r.ParseForm()
		l = modelo.Lead{Nome: campo(r, "nome"), Email: campo(r, "email"), Telefone: campo(r, "telefone"), Empresa: campo(r, "empresa"),
			Segmento: campo(r, "segmento"), Cidade: campo(r, "cidade"), Usuarios: campoInt(r, "usuarios", 0), PlanoInteresse: campo(r, "plano"), Mensagem: campo(r, "mensagem")}
	}
	l.Nome, l.Email, l.Origem = strings.TrimSpace(l.Nome), strings.ToLower(strings.TrimSpace(l.Email)), "landing"
	if l.Nome == "" || !strings.Contains(l.Email, "@") {
		s.json(w, http.StatusUnprocessableEntity, map[string]string{"erro": "informe nome e e-mail válidos"})
		return
	}
	if len(l.Mensagem) > 2000 {
		l.Mensagem = l.Mensagem[:2000]
	}
	id, err := s.repo.CriarLead(r.Context(), l)
	if err != nil {
		slog.Error("criando lead", "erro", err)
		s.json(w, http.StatusInternalServerError, map[string]string{"erro": "não foi possível registrar"})
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	s.json(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
}

// -----------------------------------------------------------------------------
// Checkout do cartão — página pública com a marca do produto
// -----------------------------------------------------------------------------

func (s *Servidor) telaCheckout(w http.ResponseWriter, r *http.Request) {
	codigo := chi.URLParam(r, "codigo")
	d := s.base(r, "Pagamento", "checkout")
	ck, err := s.ops.Checkout(r.Context(), codigo)
	if err != nil {
		d.Erro = err.Error()
		w.WriteHeader(http.StatusNotFound)
		s.render(w, r, "assinar", d)
		return
	}
	d.V["Checkout"] = ck
	d.V["Codigo"] = codigo
	d.V["Concluido"] = ck.Licenca.PagamentoStatus == modelo.PagamentoAtivo
	d.V["Simulado"] = s.ops.Sign().Simulado()
	d.V["EmailSuporte"] = s.cfg.EmailSuporte
	d.V["Form"] = map[string]string{}
	s.render(w, r, "assinar", d)
}

func (s *Servidor) processarCheckout(w http.ResponseWriter, r *http.Request) {
	codigo := chi.URLParam(r, "codigo")
	d := s.base(r, "Pagamento", "checkout")
	ck, err := s.ops.Checkout(r.Context(), codigo)
	if err != nil {
		d.Erro = err.Error()
		w.WriteHeader(http.StatusNotFound)
		s.render(w, r, "assinar", d)
		return
	}
	d.V["Checkout"], d.V["Codigo"], d.V["Simulado"] = ck, codigo, s.ops.Sign().Simulado()
	d.V["EmailSuporte"] = s.cfg.EmailSuporte
	cartao := sign.Cartao{
		Titular: campo(r, "titular"), Documento: campo(r, "documento"), Numero: campo(r, "numero"), Validade: campo(r, "validade"),
		CVV: r.PostFormValue("cvv"), IP: ipDe(r), CEP: campo(r, "cep"), Logradouro: campo(r, "logradouro"), Numero_: campo(r, "numero_endereco"),
		Complemento: campo(r, "complemento"), Bairro: campo(r, "bairro"), Cidade: campo(r, "cidade"), UF: campo(r, "uf"),
	}
	// O formulário volta preenchido só com o que não é dado de cartão.
	d.V["Form"] = map[string]string{"titular": cartao.Titular, "documento": cartao.Documento, "cep": cartao.CEP, "logradouro": cartao.Logradouro,
		"numero_endereco": cartao.Numero_, "complemento": cartao.Complemento, "bairro": cartao.Bairro, "cidade": cartao.Cidade, "uf": cartao.UF}
	if !campoBool(r, "aceite") {
		d.Erro = "Para continuar, confirme que leu e aceita as condições da assinatura."
		s.render(w, r, "assinar", d)
		return
	}
	res, err := s.ops.CadastrarCartao(r.Context(), codigo, cartao)
	if err != nil {
		d.Erro = err.Error()
		s.render(w, r, "assinar", d)
		return
	}
	switch {
	case res.URL3DS != "":
		http.Redirect(w, r, res.URL3DS, http.StatusSeeOther)
	case res.Aprovado:
		http.Redirect(w, r, s.url("/assinar/"+codigo+"?ok=1"), http.StatusSeeOther)
	default:
		d.Erro = "O cartão não foi aceito: " + res.Mensagem + ". Confira os dados ou use outro cartão."
		s.render(w, r, "assinar", d)
	}
}

// webhookRecorrencia recebe avisos do serviço de cobrança. O corpo é guardado
// cru; só pagamentos confirmados com id de assinatura dão baixa em fatura.
func (s *Servidor) webhookRecorrencia(w http.ResponseWriter, r *http.Request) {
	corpo, _ := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	var ev struct {
		Evento     string          `json:"event"`
		Status     string          `json:"status"`
		Assinatura json.RawMessage `json:"signature_id"`
		Assin2     json.RawMessage `json:"assinatura_id"`
		Tid        string          `json:"tid"`
		Referencia string          `json:"reference"`
	}
	_ = json.Unmarshal(corpo, &ev)
	assinatura := idTexto(ev.Assinatura)
	if assinatura == "" {
		assinatura = idTexto(ev.Assin2)
	}
	st := strings.ToUpper(ev.Status + " " + ev.Evento)
	pago := strings.Contains(st, "PAID") || strings.Contains(st, "PAGO") || strings.Contains(st, "APROV")
	tratado := false
	if pago && assinatura != "" {
		ref := ev.Tid
		if ref == "" {
			ref = ev.Referencia
		}
		if ok, err := s.ops.PagamentoConfirmado(r.Context(), assinatura, ref); err == nil && ok {
			tratado = true
		}
	}
	s.repo.GuardarEventoCobranca(r.Context(), "webhook", corpo, tratado)
	s.json(w, http.StatusOK, map[string]any{"ok": true, "tratado": tratado})
}

func idTexto(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var str string
	if json.Unmarshal(raw, &str) == nil {
		return strings.TrimSpace(str)
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	return ""
}

// -----------------------------------------------------------------------------
// API para os ambientes dos clientes ("Meu plano") — TOKEN_INTERNO do tenant
// -----------------------------------------------------------------------------

type tokenGuardado struct {
	token string
	ate   time.Time
}

func (s *Servidor) tokenDoTenant(r *http.Request, slug string) error {
	recebido, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || recebido == "" {
		return errors.New("sem credencial")
	}
	s.tokensMu.Lock()
	guardado, temCache := s.tokens[slug]
	s.tokensMu.Unlock()
	if !temCache || time.Now().After(guardado.ate) {
		token, err := s.prov.TokenInterno(r.Context(), slug)
		if err != nil || token == "" {
			return errors.New("ambiente não encontrado")
		}
		guardado = tokenGuardado{token: token, ate: time.Now().Add(5 * time.Minute)}
		s.tokensMu.Lock()
		s.tokens[slug] = guardado
		s.tokensMu.Unlock()
	}
	if subtle.ConstantTimeCompare([]byte(recebido), []byte(guardado.token)) != 1 {
		return errors.New("credencial inválida")
	}
	return nil
}

func (s *Servidor) apiPlanoDaEmpresa(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := s.tokenDoTenant(r, slug); err != nil {
		s.json(w, http.StatusUnauthorized, map[string]string{"erro": "não autorizado"})
		return
	}
	out, err := s.ops.PlanoDaEmpresa(r.Context(), slug)
	if err != nil {
		if errors.Is(err, repo.ErrNaoEncontrado) {
			s.json(w, http.StatusNotFound, map[string]string{"erro": "empresa não encontrada"})
			return
		}
		s.json(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	s.json(w, http.StatusOK, out)
}

func (s *Servidor) apiSolicitarPlano(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := s.tokenDoTenant(r, slug); err != nil {
		s.json(w, http.StatusUnauthorized, map[string]string{"erro": "não autorizado"})
		return
	}
	var in struct {
		Tipo     string `json:"tipo"`
		PlanoID  *int64 `json:"plano_id"`
		Mensagem string `json:"mensagem"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&in); err != nil {
		s.json(w, http.StatusBadRequest, map[string]string{"erro": "corpo inválido"})
		return
	}
	if err := s.ops.SolicitarPlano(r.Context(), slug, in.Tipo, in.PlanoID, in.Mensagem); err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, operacoes.ErrSolicitacaoPendente) {
			status = http.StatusConflict
		}
		s.json(w, status, map[string]string{"erro": err.Error()})
		return
	}
	s.json(w, http.StatusCreated, map[string]any{"ok": true})
}

// apiPublicarVersao é o endpoint da pipeline de release: registra a tag e,
// se pedido, torna-a padrão e aplica a quem auto-atualiza.
func (s *Servidor) apiPublicarVersao(w http.ResponseWriter, r *http.Request) {
	tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || subtle.ConstantTimeCompare([]byte(tok), []byte(s.cfg.ProvisionadorToken)) != 1 {
		s.json(w, http.StatusUnauthorized, map[string]string{"erro": "não autorizado"})
		return
	}
	var in struct {
		Tag       string `json:"tag"`
		Descricao string `json:"descricao"`
		Changelog string `json:"changelog"`
		Estavel   *bool  `json:"estavel"`
		Padrao    bool   `json:"padrao"`
		Aplicar   bool   `json:"aplicar"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil || strings.TrimSpace(in.Tag) == "" {
		s.json(w, http.StatusBadRequest, map[string]string{"erro": "informe a tag"})
		return
	}
	ctx := r.Context()
	estavel := in.Estavel == nil || *in.Estavel
	v, err := s.repo.VersaoPorTag(ctx, in.Tag)
	if err != nil {
		id, err := s.repo.CriarVersao(ctx, modelo.Versao{Tag: in.Tag, Descricao: in.Descricao, Changelog: in.Changelog, Estavel: estavel})
		if err != nil {
			s.json(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
			return
		}
		v = modelo.Versao{ID: id, Tag: in.Tag}
	}
	atualizados, falhas := 0, 0
	if in.Padrao {
		_ = s.repo.DefinirVersaoPadrao(ctx, v.ID)
		if in.Aplicar {
			lista, _ := s.repo.ClientesParaAutoAtualizar(ctx, v.Tag)
			for _, c := range lista {
				if err := s.ops.AtualizarVersao(ctx, c.ID, v.Tag); err != nil {
					falhas++
				} else {
					atualizados++
				}
			}
		}
	}
	s.repo.Auditar(ctx, nil, "pipeline", "publicar", "versao", &v.ID, map[string]any{"tag": v.Tag, "padrao": in.Padrao, "atualizados": atualizados}, ipDe(r))
	s.json(w, http.StatusCreated, map[string]any{"id": v.ID, "tag": v.Tag, "padrao": in.Padrao, "atualizados": atualizados, "falhas": falhas})
}

// -----------------------------------------------------------------------------
// Usuários e auditoria
// -----------------------------------------------------------------------------

func (s *Servidor) listarUsuarios(w http.ResponseWriter, r *http.Request) {
	d := s.base(r, "Usuários do painel", "usuarios")
	lista, err := s.repo.ListarUsuarios(r.Context())
	if err != nil {
		d.Erro = err.Error()
	}
	d.V["Usuarios"] = lista
	d.V["Perfis"] = []modelo.Perfil{modelo.PerfilAdmin, modelo.PerfilComercial, modelo.PerfilFinanceiro, modelo.PerfilSuporte}
	s.render(w, r, "usuarios", d)
}

func (s *Servidor) salvarUsuario(w http.ResponseWriter, r *http.Request) {
	u := modelo.Usuario{Nome: campo(r, "nome"), Email: strings.ToLower(campo(r, "email")), Perfil: modelo.Perfil(campo(r, "perfil")), Ativo: campoBool(r, "ativo")}
	if u.Nome == "" || !strings.Contains(u.Email, "@") || !u.Perfil.Valido() {
		s.redirecionar(w, r, "/usuarios", "erro", "Preencha nome, e-mail e perfil.")
		return
	}
	if id := campoInt64Ptr(r, "id"); id != nil {
		u.ID = *id
		if err := s.repo.AtualizarUsuario(r.Context(), u); err != nil {
			s.redirecionar(w, r, "/usuarios", "erro", "Não foi possível salvar: "+err.Error())
			return
		}
		s.auditar(r, "editar", "usuario", &u.ID, nil)
		s.redirecionar(w, r, "/usuarios", "ok", "Usuário atualizado.")
		return
	}
	senha := r.PostFormValue("senha")
	if err := auth.ForcaSenhaOK(senha); err != nil {
		s.redirecionar(w, r, "/usuarios", "erro", err.Error())
		return
	}
	hash, _ := auth.HashSenha(senha)
	u.SenhaHash, u.Ativo = hash, true
	id, err := s.repo.CriarUsuario(r.Context(), u)
	if err != nil {
		s.redirecionar(w, r, "/usuarios", "erro", "Não foi possível criar (e-mail já usado?).")
		return
	}
	s.auditar(r, "criar", "usuario", &id, map[string]any{"perfil": u.Perfil})
	s.redirecionar(w, r, "/usuarios", "ok", "Usuário criado.")
}

func (s *Servidor) trocarSenhaUsuario(w http.ResponseWriter, r *http.Request) {
	id := idDaRota(r)
	senha := r.PostFormValue("senha")
	if err := auth.ForcaSenhaOK(senha); err != nil {
		s.redirecionar(w, r, "/usuarios", "erro", err.Error())
		return
	}
	hash, _ := auth.HashSenha(senha)
	if err := s.repo.AtualizarSenha(r.Context(), id, hash); err != nil {
		s.redirecionar(w, r, "/usuarios", "erro", err.Error())
		return
	}
	s.auditar(r, "trocar_senha", "usuario", &id, nil)
	s.redirecionar(w, r, "/usuarios", "ok", "Senha trocada; as sessões antigas desse usuário caem.")
}

func (s *Servidor) listarAuditoria(w http.ResponseWriter, r *http.Request) {
	d := s.base(r, "Auditoria", "auditoria")
	limite, _ := strconv.Atoi(r.URL.Query().Get("limite"))
	if limite <= 0 || limite > 2000 {
		limite = 300
	}
	lista, err := s.repo.ListarAuditoria(r.Context(), limite)
	if err != nil {
		d.Erro = err.Error()
	}
	d.V["Auditoria"] = lista
	s.render(w, r, "auditoria", d)
}
