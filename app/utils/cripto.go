package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
)

// chaveCripto devolve os 32 bytes da chave de cifra do ambiente. CHAVE_CRIPTO
// (64 hex) é o que o provisionador injeta; sem ela, deriva-se do JWT_SECRET
// para o desenvolvimento avulso não quebrar — em produção a chave própria é
// obrigatória e distinta por cliente.
func chaveCripto() ([]byte, error) {
	if len(Cfg.ChaveCripto) == 64 {
		return hex.DecodeString(Cfg.ChaveCripto)
	}
	if Cfg.JWTSecret == "" {
		return nil, errors.New("CHAVE_CRIPTO (ou JWT_SECRET) não configurada")
	}
	soma := sha256.Sum256([]byte("crmia-cripto:" + Cfg.JWTSecret))
	return soma[:], nil
}

// Cifrar protege um segredo (senha SMTP, chave de API) para guardar no banco.
// AES-256-GCM, nonce aleatório prefixado, saída em base64.
func Cifrar(texto string) (string, error) {
	if texto == "" {
		return "", nil
	}
	chave, err := chaveCripto()
	if err != nil {
		return "", err
	}
	bloco, err := aes.NewCipher(chave)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(bloco)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	saida := gcm.Seal(nonce, nonce, []byte(texto), nil)
	return base64.StdEncoding.EncodeToString(saida), nil
}

// Decifrar desfaz Cifrar.
func Decifrar(cifrado string) (string, error) {
	if cifrado == "" {
		return "", nil
	}
	chave, err := chaveCripto()
	if err != nil {
		return "", err
	}
	dados, err := base64.StdEncoding.DecodeString(cifrado)
	if err != nil {
		return "", err
	}
	bloco, err := aes.NewCipher(chave)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(bloco)
	if err != nil {
		return "", err
	}
	if len(dados) < gcm.NonceSize() {
		return "", errors.New("segredo cifrado inválido")
	}
	texto, err := gcm.Open(nil, dados[:gcm.NonceSize()], dados[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("não foi possível decifrar o segredo (a chave do ambiente mudou?)")
	}
	return string(texto), nil
}
