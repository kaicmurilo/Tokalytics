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
	cookieDir   string // relative to ~/Library/Application Support (macOS) or ~/.config (Linux)
	keychainKey string // macOS Keychain service name (unused on Linux)
}

var chromiumBrowsersMac = []browserProfile{
	{"Arc", "Arc/User Data", "Arc Safe Storage"},
	{"Chrome", "Google/Chrome", "Chrome Safe Storage"},
	{"Brave", "BraveSoftware/Brave-Browser", "Brave Safe Storage"},
	{"Microsoft Edge", "Microsoft Edge", "Microsoft Edge Safe Storage"},
	{"Vivaldi", "Vivaldi", "Vivaldi Safe Storage"},
	{"Chromium", "Chromium", "Chromium Safe Storage"},
	{"Opera", "com.operasoftware.Opera", "Opera Safe Storage"},
}

// chromiumBrowsersLinux lists profile dirs relative to ~/.config
var chromiumBrowsersLinux = []browserProfile{
	{"Chrome", "google-chrome", ""},
	{"Chromium", "chromium", ""},
	{"Brave", "BraveSoftware/Brave-Browser", ""},
	{"Microsoft Edge", "microsoft-edge", ""},
	{"Vivaldi", "vivaldi", ""},
	{"Opera", "opera", ""},
}

var profileDirs = []string{"Default", "Profile 1", "Profile 2", "Profile 3"}

// GetChromeCookie tries all installed Chromium-based browsers.
// macOS: uses Keychain-based AES decryption.
// Linux: uses the "peanuts" fallback key (Chrome's default when no keyring is available).
func GetChromeCookie(domain, name string) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return getChromeCookieDarwin(domain, name)
	case "linux":
		return getChromeCookieLinux(domain, name)
	default:
		return "", fmt.Errorf("unsupported OS")
	}
}

func getChromeCookieDarwin(domain, name string) (string, error) {
	home, _ := os.UserHomeDir()
	appSupport := filepath.Join(home, "Library", "Application Support")

	var lastErr error
	for _, browser := range chromiumBrowsersMac {
		browserBase := filepath.Join(appSupport, browser.cookieDir)
		if _, err := os.Stat(browserBase); os.IsNotExist(err) {
			continue
		}

		key, err := getChromiumKey(browser.name, browser.keychainKey)
		if err != nil {
			lastErr = fmt.Errorf("[%s] keychain: %v", browser.name, err)
			continue
		}

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

// getChromeCookieLinux tries Chromium browsers using the "peanuts" fallback key.
// This works when Chrome runs without GNOME Keyring / KWallet (common in dev setups).
func getChromeCookieLinux(domain, name string) (string, error) {
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".config")

	// "peanuts" is Chrome's hardcoded fallback password on Linux (1 PBKDF2 iteration).
	key := pbkdf2.Key([]byte("peanuts"), []byte("saltysalt"), 1, 16, sha1.New)

	var lastErr error
	for _, browser := range chromiumBrowsersLinux {
		browserBase := filepath.Join(configDir, browser.cookieDir)
		if _, err := os.Stat(browserBase); os.IsNotExist(err) {
			continue
		}

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

// cursorAppDataDir retorna o diretório de dados do Cursor (Electron) por plataforma.
func cursorAppDataDir() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "linux":
		return filepath.Join(home, ".config", "Cursor")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Cursor")
	case "windows":
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, "Cursor")
		}
		return filepath.Join(home, "AppData", "Roaming", "Cursor")
	default:
		return ""
	}
}

// cursorAppDataDirs lista pastas do Cursor que podem conter User/globalStorage/state.vscdb
// (~/.config/Cursor no Linux; em WSL também AppData\Roaming\Cursor do Windows).
func cursorAppDataDirs() []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		p = filepath.Clean(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if main := cursorAppDataDir(); main != "" {
		add(main)
	}
	if runtime.GOOS == "linux" && IsWSL() {
		if wh := WindowsHomeOnWSL(); wh != "" {
			add(filepath.Join(wh, "AppData", "Roaming", "Cursor"))
		}
	}
	return out
}

func scanCursorAccessTokenSQLite(tmpPath string) (string, error) {
	db, err := sql.Open("sqlite3", tmpPath+"?immutable=1")
	if err != nil {
		return "", fmt.Errorf("open cursor db: %v", err)
	}
	defer db.Close()
	var token string
	err = db.QueryRow(`SELECT value FROM ItemTable WHERE key = 'cursorAuth/accessToken' LIMIT 1`).Scan(&token)
	if err != nil {
		return "", fmt.Errorf("cursor accessToken not found: %v", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("cursor accessToken is empty")
	}
	return token, nil
}

// GetCursorAuthToken lê o accessToken do Cursor diretamente do banco SQLite local.
// Funciona em todas as plataformas sem precisar de cookie manual.
func GetCursorAuthToken() (string, error) {
	dirs := cursorAppDataDirs()
	if len(dirs) == 0 {
		return "", fmt.Errorf("unsupported OS")
	}
	var lastErr error
	for _, appDir := range dirs {
		dbPath := filepath.Join(appDir, "User", "globalStorage", "state.vscdb")
		if _, err := os.Stat(dbPath); err != nil {
			lastErr = fmt.Errorf("cursor db not found: %s", dbPath)
			continue
		}
		data, err := os.ReadFile(dbPath)
		if err != nil {
			lastErr = fmt.Errorf("read cursor db: %v", err)
			continue
		}
		tmpF, err := os.CreateTemp("", "tokalytics-cursor-*.db")
		if err != nil {
			lastErr = err
			continue
		}
		tmpPath := tmpF.Name()
		_ = tmpF.Close()
		if err := os.WriteFile(tmpPath, data, 0600); err != nil {
			_ = os.Remove(tmpPath)
			lastErr = fmt.Errorf("write tmp cursor db: %v", err)
			continue
		}
		token, err := scanCursorAccessTokenSQLite(tmpPath)
		_ = os.Remove(tmpPath)
		if err != nil {
			lastErr = err
			continue
		}
		return token, nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("cursor accessToken not found in any Cursor data directory")
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

	// Sanitize: keep only printable ASCII (32-126), excluding DEL (0x7F) and non-ASCII.
	// This ensures the result is valid as an HTTP header value.
	result := strings.Map(func(r rune) rune {
		if r >= 32 && r <= 126 {
			return r
		}
		return -1
	}, string(decrypted))
	result = strings.TrimSpace(result)

	if result == "" {
		return "", fmt.Errorf("decrypted value is empty after sanitization")
	}
	// Sanity check: a valid cookie value should be at least 8 chars and not contain spaces.
	// Garbage from wrong decryption key tends to look random and often includes spaces.
	if len(result) < 8 || strings.ContainsAny(result, " \t;,") {
		return "", fmt.Errorf("decrypted value looks invalid (len=%d)", len(result))
	}
	return result, nil
}
