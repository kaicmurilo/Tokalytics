package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/pbkdf2"
)

// browserProfile defines a Chromium-based browser's cookie location and keychain key
type browserProfile struct {
	name        string
	cookieDir   string // relative to ~/Library/Application Support
	keychainKey string
}

var chromiumBrowsers = []browserProfile{
	// name = Keychain account name, keychainKey = service name
	{"Arc", "Arc/User Data", "Arc Safe Storage"},
	{"Chrome", "Google/Chrome", "Chrome Safe Storage"},
	{"Brave", "BraveSoftware/Brave-Browser", "Brave Safe Storage"},
	{"Microsoft Edge", "Microsoft Edge", "Microsoft Edge Safe Storage"},
	{"Vivaldi", "Vivaldi", "Vivaldi Safe Storage"},
	{"Chromium", "Chromium", "Chromium Safe Storage"},
	{"Opera", "com.operasoftware.Opera", "Opera Safe Storage"},
}

var profileDirs = []string{"Default", "Profile 1", "Profile 2", "Profile 3"}

// GetChromeCookie tries all installed Chromium-based browsers on macOS
func GetChromeCookie(domain, name string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("unsupported OS")
	}

	home, _ := os.UserHomeDir()
	appSupport := filepath.Join(home, "Library", "Application Support")

	var lastErr error
	for _, browser := range chromiumBrowsers {
		browserBase := filepath.Join(appSupport, browser.cookieDir)
		if _, err := os.Stat(browserBase); os.IsNotExist(err) {
			continue
		}

		// Get decryption key from Keychain: account=browser.name, service=browser.keychainKey
		key, err := getChromiumKey(browser.name, browser.keychainKey)
		if err != nil {
			lastErr = fmt.Errorf("[%s] keychain: %v", browser.name, err)
			continue
		}

		// Try each profile
		for _, profile := range profileDirs {
			cookiePath := filepath.Join(browserBase, profile, "Cookies")
			if _, err := os.Stat(cookiePath); os.IsNotExist(err) {
				continue
			}

			val, err := decryptCookie(cookiePath, domain, name, key)
			if err == nil && val != "" {
				return val, nil
			}
			if err != nil {
				lastErr = fmt.Errorf("[%s/%s] %v", browser.name, profile, err)
			}
		}
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("cookie not found in any browser")
}

// getChromiumKey retrieves the AES key from macOS Keychain for a given browser.
// Keychain entry: account = browser name, service = "{Browser} Safe Storage"
func getChromiumKey(browserName, serviceName string) ([]byte, error) {
	// Correct format: -a <account> -s <service> -w
	cmd := exec.Command("security", "find-generic-password", "-a", browserName, "-s", serviceName, "-w")
	out, err := cmd.Output()
	password := strings.TrimSpace(string(out))

	if err != nil || password == "" {
		return nil, fmt.Errorf("keychain item %q not found or inaccessible: %v", serviceName, err)
	}

	// Derive AES-128 key via PBKDF2-SHA1, 1003 iterations
	key := pbkdf2.Key([]byte(password), []byte("saltysalt"), 1003, 16, sha1.New)
	return key, nil
}

func decryptCookie(cookiePath, domain, name string, key []byte) (string, error) {
	// Copy DB to temp (Chrome locks the file while running)
	tmpFile := filepath.Join(os.TempDir(), "tokalytics_cookie_tmp.db")
	data, err := os.ReadFile(cookiePath)
	if err != nil {
		return "", fmt.Errorf("read cookie db: %v", err)
	}
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return "", fmt.Errorf("write tmp db: %v", err)
	}
	defer os.Remove(tmpFile)

	db, err := sql.Open("sqlite3", tmpFile+"?immutable=1")
	if err != nil {
		return "", fmt.Errorf("open sqlite: %v", err)
	}
	defer db.Close()

	var encrypted []byte
	query := `SELECT encrypted_value FROM cookies WHERE host_key LIKE ? AND name = ? LIMIT 1`
	if err := db.QueryRow(query, "%"+domain+"%", name).Scan(&encrypted); err != nil {
		return "", fmt.Errorf("query: %v", err)
	}
	if len(encrypted) == 0 {
		return "", fmt.Errorf("empty cookie value")
	}

	// Strip v10/v11 prefix (3 bytes)
	if len(encrypted) > 3 && (string(encrypted[:3]) == "v10" || string(encrypted[:3]) == "v11") {
		encrypted = encrypted[3:]
	}

	if len(encrypted) == 0 || len(encrypted)%16 != 0 {
		return "", fmt.Errorf("invalid encrypted length %d", len(encrypted))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes: %v", err)
	}

	iv := []byte("                ") // 16 spaces
	decrypted := make([]byte, len(encrypted))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(decrypted, encrypted)

	// PKCS7 unpad
	if len(decrypted) == 0 {
		return "", fmt.Errorf("empty decryption result")
	}
	pad := int(decrypted[len(decrypted)-1])
	if pad > 0 && pad <= 16 && len(decrypted) >= pad {
		decrypted = decrypted[:len(decrypted)-pad]
	}

	// Sanitize: remove null bytes and non-printable chars
	result := strings.Map(func(r rune) rune {
		if r == 0 || r > 127 || (r < 32 && r != '\t') {
			return -1
		}
		return r
	}, string(decrypted))
	result = strings.TrimSpace(result)

	if result == "" {
		return "", fmt.Errorf("decrypted value is empty after sanitization")
	}
	return result, nil
}
