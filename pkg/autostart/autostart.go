package autostart

import (
	"fmt"
	"os"
)

// Supported indica se há implementação de login item para o SO atual.
func Supported() bool {
	return platformSupported()
}

// SetEnabled registra ou remove o app para iniciar junto com a sessão do usuário.
func SetEnabled(enable bool) error {
	if !platformSupported() {
		if enable {
			return fmt.Errorf("início automático não suportado neste sistema")
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return platformSet(enable, exe)
}
