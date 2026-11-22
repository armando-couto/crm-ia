package controllers

import "github.com/kataras/iris/v12"

// naoImplementado é a resposta das rotas de prospecção cujo handler ainda não
// foi escrito: a rota e a permissão já valem, o corpo vem depois.
func naoImplementado(ctx iris.Context) {
	ctx.StopWithJSON(iris.StatusNotImplemented, iris.Map{
		"error":  "esta função ainda não está disponível",
		"codigo": "nao_implementado",
	})
}
