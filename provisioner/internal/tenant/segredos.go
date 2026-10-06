package tenant

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
)

const alfabetoSenha = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"

// segredo gera um texto aleatório sem caracteres ambíguos (vai para senhas de
// banco e para a senha inicial do administrador, que alguém digita).
func segredo(tamanho int) (string, error) {
	out := make([]byte, tamanho)
	max := big.NewInt(int64(len(alfabetoSenha)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = alfabetoSenha[n.Int64()]
	}
	return string(out), nil
}

// chaveHex gera `bytes` bytes aleatórios em hexadecimal (JWT, cripto, tokens).
func chaveHex(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
