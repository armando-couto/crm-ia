//go:build !unix

package carga

// espacoLivre: fora de sistemas Unix não há conferência (o serviço roda em
// contêiner Linux).
func espacoLivre(string) (uint64, error) { return 1 << 62, nil }
