package tenant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// compose roda `docker compose` sobre o arquivo do cliente.
func (s *Servico) compose(ctx context.Context, slug string, args ...string) (string, error) {
	completo := append([]string{"compose", "-f", s.caminhoCompose(slug)}, args...)
	return s.dockerCom(ctx, 8*time.Minute, "", completo...)
}

func (s *Servico) docker(ctx context.Context, args ...string) (string, error) {
	return s.dockerCom(ctx, 2*time.Minute, "", args...)
}

// dockerCom executa o binário docker (o provisionador fala com o daemon pelo
// socket montado). A saída volta inteira para o log de erro ser útil.
func (s *Servico) dockerCom(ctx context.Context, prazo time.Duration, entrada string, args ...string) (string, error) {
	ctx, cancelar := context.WithTimeout(ctx, prazo)
	defer cancelar()
	cmd := exec.CommandContext(ctx, s.cfg.DockerBin(), args...)
	if entrada != "" {
		cmd.Stdin = strings.NewReader(entrada)
	}
	var saida, erro bytes.Buffer
	cmd.Stdout, cmd.Stderr = &saida, &erro
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(erro.String())
		if msg == "" {
			msg = strings.TrimSpace(saida.String())
		}
		return saida.String(), fmt.Errorf("docker %s: %s", args[0], limitar(msg, 600))
	}
	return saida.String(), nil
}

// garantirImagem baixa a imagem se ela ainda não está no host. Tag mutável
// (latest) é sempre puxada de novo — pode ter sido reconstruída.
func (s *Servico) garantirImagem(ctx context.Context, referencia string) error {
	if !tagMutavel(referencia) {
		if _, err := s.docker(ctx, "image", "inspect", referencia); err == nil {
			return nil
		}
	}
	if s.cfg.RegistroUsuario != "" && s.cfg.RegistroToken != "" {
		registro := registroDaImagem(referencia)
		if _, err := s.dockerCom(ctx, time.Minute, s.cfg.RegistroToken, "login", registro, "-u", s.cfg.RegistroUsuario, "--password-stdin"); err != nil {
			slog.Warn("login no registro falhou; tentando pull mesmo assim", "registro", registro, "erro", err)
		}
	}
	if _, err := s.dockerCom(ctx, 10*time.Minute, "", "pull", referencia); err != nil {
		if _, e := s.docker(ctx, "image", "inspect", referencia); e == nil {
			slog.Warn("pull falhou, usando a imagem já presente no host", "imagem", referencia, "erro", err)
			return nil
		}
		return fmt.Errorf("imagem %s indisponível: %w", referencia, err)
	}
	return nil
}

func tagMutavel(referencia string) bool {
	i := strings.LastIndex(referencia, ":")
	return i < 0 || strings.Contains(referencia[i:], "/") || referencia[i+1:] == "latest"
}

func registroDaImagem(referencia string) string {
	partes := strings.SplitN(referencia, "/", 2)
	if len(partes) == 2 && (strings.Contains(partes[0], ".") || strings.Contains(partes[0], ":")) {
		return partes[0]
	}
	return "docker.io"
}

func (s *Servico) garantirImagensDaVersao(ctx context.Context, versao string) error {
	for _, img := range []string{s.cfg.ImagemDB, s.cfg.ImagemRedis, s.cfg.ImagemApp + ":" + versao} {
		if err := s.garantirImagem(ctx, img); err != nil {
			return err
		}
	}
	return nil
}

// inspecao é o pedaço do `docker inspect` que interessa.
type inspecao struct {
	State struct {
		Status string `json:"Status"`
		Health *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	Config struct {
		Image  string            `json:"Image"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	Image string `json:"Image"`
}

func (s *Servico) inspecionar(ctx context.Context, nome string) (inspecao, bool) {
	saida, err := s.docker(ctx, "inspect", nome)
	if err != nil {
		return inspecao{}, false
	}
	var lista []inspecao
	if json.Unmarshal([]byte(saida), &lista) != nil || len(lista) == 0 {
		return inspecao{}, false
	}
	return lista[0], true
}

// estadoContainer devolve running/exited/… ou "" quando não existe.
func (s *Servico) estadoContainer(ctx context.Context, nome string) string {
	i, ok := s.inspecionar(ctx, nome)
	if !ok {
		return ""
	}
	return i.State.Status
}

// esperarSaudavel aguarda o healthcheck do container ficar "healthy".
func (s *Servico) esperarSaudavel(ctx context.Context, nome string, limite time.Duration) error {
	fim := time.Now().Add(limite)
	for {
		i, ok := s.inspecionar(ctx, nome)
		if ok {
			if i.State.Health != nil {
				switch i.State.Health.Status {
				case "healthy":
					return nil
				case "unhealthy":
					logs, _ := s.docker(ctx, "logs", "--tail", "40", nome)
					return fmt.Errorf("container %s não ficou saudável:\n%s", nome, limitar(logs, 1500))
				}
			} else if i.State.Status == "running" {
				return nil // sem healthcheck: rodando basta
			}
			if i.State.Status == "exited" || i.State.Status == "dead" {
				logs, _ := s.docker(ctx, "logs", "--tail", "40", nome)
				return fmt.Errorf("container %s parou durante a subida:\n%s", nome, limitar(logs, 1500))
			}
		}
		if time.Now().After(fim) {
			return fmt.Errorf("container %s não respondeu em %s", nome, limite)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.intervaloEspera):
		}
	}
}

// containersDoTenant lista os containers da stack com estado e imagem.
func (s *Servico) containersDoTenant(ctx context.Context, slug string) []ContainerInfo {
	saida, err := s.docker(ctx, "ps", "-a", "--filter", "label=crmia.tenant="+slug, "--format", "{{.Names}}\t{{.State}}\t{{.Status}}\t{{.Image}}")
	if err != nil {
		return nil
	}
	var lista []ContainerInfo
	for _, linha := range strings.Split(strings.TrimSpace(saida), "\n") {
		campos := strings.Split(linha, "\t")
		if len(campos) < 4 || campos[0] == "" {
			continue
		}
		lista = append(lista, ContainerInfo{Nome: campos[0], Estado: campos[1], Status: campos[2], Imagem: campos[3]})
	}
	return lista
}

// versaoEmExecucao lê a tag que o container do app realmente roda.
func (s *Servico) versaoEmExecucao(ctx context.Context, slug string) string {
	i, ok := s.inspecionar(ctx, prefixo+slug+"-app")
	if !ok {
		return ""
	}
	if v := i.Config.Labels["crmia.versao"]; v != "" {
		return v
	}
	if idx := strings.LastIndex(i.Config.Image, ":"); idx >= 0 {
		return i.Config.Image[idx+1:]
	}
	return ""
}

func limitar(v string, max int) string {
	v = strings.TrimSpace(v)
	if len(v) <= max {
		return v
	}
	return v[:max] + "…"
}
