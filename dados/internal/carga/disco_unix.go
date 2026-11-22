//go:build unix

package carga

import "syscall"

// espacoLivre devolve os bytes livres no volume do diretório.
func espacoLivre(dir string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil //nolint:unconvert // os tipos mudam entre Linux e macOS
}
