//go:build !windows

package utils

func getChromeCookieWindows(domain, name string) (string, error) {
	return "", nil
}
