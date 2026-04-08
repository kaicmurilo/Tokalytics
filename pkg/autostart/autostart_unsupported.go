//go:build !darwin && !linux && !windows

package autostart

import "fmt"

func platformSupported() bool { return false }

func platformSet(enable bool, exe string) error {
	if enable {
		return fmt.Errorf("início automático não suportado neste sistema")
	}
	return nil
}
