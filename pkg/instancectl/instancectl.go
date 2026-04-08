package instancectl

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const (
	ServiceName  = "tokalytics"
	healthPath   = "/api/health"
	refreshPath  = "/api/refresh"
	shutdownPath = "/api/shutdown"
)

// PortMin e PortMax delimitam a faixa onde o dashboard HTTP do Tokalytics é procurado.
const PortMin, PortMax = 3456, 3555

var httpClient = &http.Client{Timeout: 800 * time.Millisecond}

type healthBody struct {
	Service string `json:"service"`
	Version string `json:"version"`
}

// RunningInfo retorna porta e versão reportada por /api/health quando o Tokalytics responde.
func RunningInfo() (port int, apiVersion string, ok bool) {
	for p := PortMin; p <= PortMax; p++ {
		if hok, ver := checkHealth(p); hok {
			return p, ver, true
		}
	}
	return 0, "", false
}

// FindRunning escaneia portas típicas e retorna a porta se responder como Tokalytics.
func FindRunning() (port int, ok bool) {
	p, _, ok := RunningInfo()
	return p, ok
}

func checkHealth(port int) (ok bool, version string) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", port, healthPath)
	resp, err := httpClient.Get(url)
	if err != nil {
		return false, ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, ""
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if err != nil {
		return false, ""
	}
	var h healthBody
	if json.Unmarshal(b, &h) != nil || h.Service != ServiceName {
		return false, ""
	}
	return true, h.Version
}

// PortFromRunstate tenta o port gravado; valida com /api/health.
func PortFromRunstate(rsPort int) (port int, ok bool) {
	if rsPort < PortMin || rsPort > PortMax {
		return 0, false
	}
	ok, _ = checkHealth(rsPort)
	if ok {
		return rsPort, true
	}
	return 0, false
}

// Reload chama o refresh da instância na porta indicada.
func Reload(port int) error {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", port, refreshPath)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("não foi possível contatar Tokalytics na porta %d: %w", port, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("refresh retornou HTTP %d", resp.StatusCode)
	}
	return nil
}

// Shutdown pede encerramento gracioso via API (apenas loopback).
func Shutdown(port int) error {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", port, shutdownPath)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("não foi possível contatar Tokalytics na porta %d: %w", port, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("shutdown retornou HTTP %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// LoopbackRequest retorna true se a requisição vier só de loopback.
func LoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
