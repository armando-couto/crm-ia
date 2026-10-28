package services

import (
	"log"
	"sync"
)

// tarefas acompanha o trabalho disparado em segundo plano pelos controllers
// (notificações, gatilhos de automação). Em produção ninguém espera; nos
// testes, Aguardar garante que nada continue rodando depois que o estado
// global (banco, mailer) for trocado.
var tarefas sync.WaitGroup

// EmSegundoPlano roda fn numa goroutine acompanhada, sem derrubar o
// processo se ela entrar em pânico.
func EmSegundoPlano(fn func()) {
	tarefas.Add(1)
	go func() {
		defer tarefas.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("tarefa em segundo plano falhou: %v", r)
			}
		}()
		fn()
	}()
}

// AguardarSegundoPlano bloqueia até todas as tarefas terminarem.
func AguardarSegundoPlano() { tarefas.Wait() }
