//go:build windows

package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/sys/windows"
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

var (
	crypt32DLL         = windows.NewLazySystemDLL("crypt32.dll")
	cryptUnprotectData = crypt32DLL.NewProc("CryptUnprotectData")
	kernel32DLL        = windows.NewLazySystemDLL("kernel32.dll")
	localFreeProc      = kernel32DLL.NewProc("LocalFree")
)

func decryptDPAPI(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("dpapi: empty input")
	}
	var in, out dataBlob
	in.cbData = uint32(len(data))
	in.pbData = &data[0]
	r, _, err := cryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, fmt.Errorf("CryptUnprotectData: %w", err)
	}
	defer localFreeProc.Call(uintptr(unsafe.Pointer(out.pbData)))
	result := make([]byte, out.cbData)
	copy(result, unsafe.Slice(out.pbData, out.cbData))
	return result, nil
}

func windowsBrowserList() []struct{ name, userDataDir string } {
	local := os.Getenv("LOCALAPPDATA")
	return []struct{ name, userDataDir string }{
		{"Chrome", filepath.Join(local, "Google", "Chrome", "User Data")},
		{"Edge", filepath.Join(local, "Microsoft", "Edge", "User Data")},
		{"Brave", filepath.Join(local, "BraveSoftware", "Brave-Browser", "User Data")},
	}
}

func getChromeCookieWindows(domain, name string) (string, error) {
	var lastErr error
	for _, browser := range windowsBrowserList() {
		if _, err := os.Stat(browser.userDataDir); err != nil {
			continue
		}
		aesKey, err := readWindowsBrowserKey(browser.userDataDir)
		if err != nil {
			lastErr = fmt.Errorf("[%s] key: %v", browser.name, err)
			continue
		}
		for _, profile := range profileDirs {
			// Chrome 96+ stores cookies under Network/Cookies; older versions use Cookies directly.
			for _, rel := range []string{filepath.Join("Network", "Cookies"), "Cookies"} {
				cookiePath := filepath.Join(browser.userDataDir, profile, rel)
				if _, err := os.Stat(cookiePath); err != nil {
					continue
				}
				val, err := decryptWindowsCookie(cookiePath, domain, name, aesKey)
				if err == nil && val != "" {
					return val, nil
				}
				if err != nil {
					lastErr = fmt.Errorf("[%s/%s] %v", browser.name, profile, err)
				}
			}
		}
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("cookie %q not found in any Windows browser", name)
}

func readWindowsBrowserKey(userDataDir string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(userDataDir, "Local State"))
	if err != nil {
		return nil, fmt.Errorf("read Local State: %v", err)
	}
	var ls struct {
		OsCrypt struct {
			EncryptedKey string `json:"encrypted_key"`
		} `json:"os_crypt"`
	}
	if err := json.Unmarshal(data, &ls); err != nil || ls.OsCrypt.EncryptedKey == "" {
		return nil, fmt.Errorf("encrypted_key not found in Local State")
	}
	keyEnc, err := base64.StdEncoding.DecodeString(ls.OsCrypt.EncryptedKey)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %v", err)
	}
	if len(keyEnc) <= 5 {
		return nil, fmt.Errorf("encrypted key too short")
	}
	return decryptDPAPI(keyEnc[5:]) // strip 5-byte "DPAPI" prefix
}

func decryptWindowsCookie(cookiePath, domain, name string, aesKey []byte) (string, error) {
	data, err := os.ReadFile(cookiePath)
	if err != nil {
		return "", fmt.Errorf("read cookie db: %v", err)
	}
	tmp, err := os.CreateTemp("", "tokalytics-wcookie-*.db")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	defer os.Remove(tmpPath)

	db, err := sql.Open("sqlite3", tmpPath+"?immutable=1")
	if err != nil {
		return "", fmt.Errorf("open cookie db: %v", err)
	}
	defer db.Close()

	var encrypted []byte
	if err := db.QueryRow(
		"SELECT encrypted_value FROM cookies WHERE host_key LIKE ? AND name = ? LIMIT 1",
		"%"+domain+"%", name,
	).Scan(&encrypted); err != nil {
		return "", fmt.Errorf("query: %v", err)
	}

	// Format: 3-byte prefix (v10/v11) + 12-byte nonce + AES-256-GCM ciphertext+tag
	if len(encrypted) < 3+12+1 {
		return "", fmt.Errorf("cookie too short (%d bytes)", len(encrypted))
	}
	prefix := string(encrypted[:3])
	if prefix != "v10" && prefix != "v11" {
		return "", fmt.Errorf("unknown cookie format %q", prefix)
	}
	nonce := encrypted[3:15]
	ciphertext := encrypted[15:]

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", fmt.Errorf("aes cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm: %v", err)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("gcm decrypt: %v", err)
	}
	return string(plaintext), nil
}
