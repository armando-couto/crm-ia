package web

import (
	"sync"
	"time"
)

// limitador conta requisições por chave numa janela deslizante, em memória.
// O painel roda com uma réplica e o contador zera a cada reinício; para o
// papel que tem aqui (conter rajadas de um ambiente) isso basta. O valor zero
// já está pronto para uso.
type limitador struct {
	mu     sync.Mutex
	marcas map[string][]time.Time
	usos   int
}

// permitir registra uma requisição da chave, se couber. Quando não cabe,
// devolve quanto falta para a marca mais antiga sair da janela.
func (l *limitador) permitir(chave string, limite int, janela time.Duration) (bool, time.Duration) {
	return l.permitirEm(time.Now(), chave, limite, janela)
}

func (l *limitador) permitirEm(agora time.Time, chave string, limite int, janela time.Duration) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.marcas == nil {
		l.marcas = map[string][]time.Time{}
	}
	corte := agora.Add(-janela)
	// De tempos em tempos, solta as chaves que ficaram sem marca viva.
	if l.usos++; l.usos%4096 == 0 {
		for k, m := range l.marcas {
			if len(m) == 0 || !m[len(m)-1].After(corte) {
				delete(l.marcas, k)
			}
		}
	}
	marcas := l.marcas[chave]
	i := 0
	for i < len(marcas) && !marcas[i].After(corte) {
		i++
	}
	marcas = marcas[i:]
	if len(marcas) >= limite {
		l.marcas[chave] = marcas
		espera := marcas[0].Add(janela).Sub(agora)
		if espera < time.Second {
			espera = time.Second
		}
		return false, espera
	}
	l.marcas[chave] = append(marcas, agora)
	return true, 0
}
