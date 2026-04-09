#!/usr/bin/env bash
# Garante um toolchain Go disponível (PATH ou instalação local em ~/.local/share/tokalytics-go).
# Suporta Linux (amd64/arm64) e macOS (amd64/arm64).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GO_VERSION="${TOKALYTICS_GO_VERSION:-1.26.2}"
INSTALL_ROOT="${TOKALYTICS_GO_INSTALL:-$HOME/.local/share/tokalytics-go}"
GO_DIR="$INSTALL_ROOT/go"

_OS="$(uname -s)"
_ARCH="$(uname -m)"

go_platform_target() {
  case "$_OS" in
    Linux)
      case "$_ARCH" in
        x86_64)          echo "linux-amd64" ;;
        aarch64 | arm64) echo "linux-arm64" ;;
        *) echo "" ;;
      esac
      ;;
    Darwin)
      case "$_ARCH" in
        x86_64) echo "darwin-amd64" ;;
        arm64)  echo "darwin-arm64" ;;
        *) echo "" ;;
      esac
      ;;
    *) echo "" ;;
  esac
}

GO_TARGET="${TOKALYTICS_GO_TARGET:-$(go_platform_target)}"
if [[ -z "$GO_TARGET" ]]; then
  echo "Tokalytics: arquitetura não suportada para download automático do Go ($_OS/$_ARCH). Instale Go manualmente: https://go.dev/dl/" >&2
  exit 1
fi

ARCHIVE="go${GO_VERSION}.${GO_TARGET}.tar.gz"
URL="https://go.dev/dl/${ARCHIVE}"

# SHA256 oficial por plataforma/arquitetura (go.dev/dl); sobrescreva com TOKALYTICS_GO_SHA256 se necessário.
# darwin-amd64 / darwin-arm64: defina TOKALYTICS_GO_SHA256 ou deixe vazio para pular verificação.
if [[ -z "${TOKALYTICS_GO_SHA256:-}" ]]; then
  case "$GO_TARGET" in
    linux-amd64)  EXPECTED_SHA256="990e6b4bbba816dc3ee129eaeaf4b42f17c2800b88a2166c265ac1a200262282" ;;
    linux-arm64)  EXPECTED_SHA256="c958a1fe1b361391db163a485e21f5f228142d6f8b584f6bef89b26f66dc5b23" ;;
    darwin-amd64) EXPECTED_SHA256="" ;;
    darwin-arm64) EXPECTED_SHA256="" ;;
    *) EXPECTED_SHA256="" ;;
  esac
else
  EXPECTED_SHA256="$TOKALYTICS_GO_SHA256"
fi

sha256_verify() {
  local expected="$1" file="$2"
  if [[ -z "$expected" ]]; then
    return 0  # sem hash conhecido, pular verificação
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    echo "$expected  $file" | sha256sum -c -
  elif command -v shasum >/dev/null 2>&1; then
    echo "$expected  $file" | shasum -a 256 -c -
  else
    echo "Tokalytics: aviso — nenhuma ferramenta de SHA256 encontrada; pulando verificação." >&2
  fi
}

need_download() {
  if [[ ! -x "$GO_DIR/bin/go" ]]; then
    return 0
  fi
  local ver
  ver="$("$GO_DIR/bin/go" version 2>/dev/null | awk '{print $3}' || true)"
  [[ "$ver" == "go${GO_VERSION}" ]] || return 0
  return 1
}

download_and_extract() {
  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  echo "Tokalytics: Go não encontrado no PATH. Baixando Go ${GO_VERSION} para ${GO_DIR} ..."
  mkdir -p "$INSTALL_ROOT"
  curl -fsSL "$URL" -o "$tmp/$ARCHIVE"
  sha256_verify "$EXPECTED_SHA256" "$tmp/$ARCHIVE"
  rm -rf "$GO_DIR"
  tar -C "$INSTALL_ROOT" -xzf "$tmp/$ARCHIVE"
}

ensure_linux_cgo_deps() {
  if [[ "$_OS" != Linux ]] || ! command -v pkg-config >/dev/null 2>&1; then
    return 0
  fi
  if ! pkg-config --exists ayatana-appindicator3-0.1 2>/dev/null; then
    echo "Tokalytics: falta a biblioteca de desenvolvimento do system tray (CGO)." >&2
    echo "  Debian/Ubuntu/Mint: sudo apt install libayatana-appindicator3-dev build-essential pkg-config" >&2
    exit 1
  fi
}

if command -v go >/dev/null 2>&1; then
  cd "$ROOT"
  ensure_linux_cgo_deps
  exec go "$@"
fi

export PATH="$GO_DIR/bin:$PATH"

if need_download; then
  download_and_extract
  export PATH="$GO_DIR/bin:$PATH"
fi

if ! command -v go >/dev/null 2>&1; then
  echo "Tokalytics: não foi possível localizar ou instalar o Go. Instale manualmente: https://go.dev/dl/" >&2
  exit 1
fi

cd "$ROOT"
ensure_linux_cgo_deps
exec go "$@"
