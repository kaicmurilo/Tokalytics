# Tokalytics

Ferramenta **local** (Go) que agrega uso de tokens e quotas: **menu bar** (system tray) + **dashboard web** para Claude Code, Cursor, Gemini CLI, Codex e dados em nuvem (Claude), quando configurado.

Os dados permanecem na sua máquina; nada é enviado para servidores externos pelo app.

## Requisitos

- [Go](https://go.dev/dl/) 1.21+ (para build / `go run`)
- Node/npm opcional — apenas atalhos em `package.json` para os mesmos comandos Go

## Como executar

### Desenvolvimento (sem gerar binário)

```bash
npm run dev
# ou:
go run . -dev
```

O flag **`-dev`** (ou a variável **`TOKALYTICS_DEV=1`**) faz o app **ignorar** outra instância já em execução (por exemplo a instalada via `npm install -g`). O dashboard sobe na **próxima porta livre** (ex.: `3457` se `3456` estiver com o app global).

Sem `-dev`, se já existir Tokalytics na faixa de portas, o processo apenas informa a URL e encerra.

### Build em Go (binário `tokalytics`)

Na raiz do repositório:

```bash
go build -o tokalytics main.go
```

Isso gera o executável `tokalytics` no diretório atual. Para rodar o app após o build:

```bash
./tokalytics
```

### Atalhos npm (equivalentes)

```bash
npm run dev      # go run . -dev
npm run build    # go build -o tokalytics main.go
npm run start    # compila e em seguida executa ./tokalytics
```

Na primeira execução o app sobe o **servidor HTTP** (porta padrão `3456`; se estiver ocupada, usa a próxima livre) e o ícone na barra de menus. Use **«Abrir Dashboard»** no menu ou abra a URL mostrada no terminal.

### Instalação global pelo npm

```bash
npm install -g tokalytics
```

Em instalação **global**, o `postinstall` tenta **abrir o Tokalytics em segundo plano** (ícone na barra). Para instalar sem iniciar: `TOKALYTICS_NO_AUTOSTART=1 npm install -g tokalytics`.

Instalação a partir do repositório (sempre o `postinstall` da branch atual):

```bash
npm install -g "github:kaicmurilo/Tokalytics"
```

### CLI: instância única e controle

O binário `tokalytics` evita subir uma **segunda** cópia: se já houver uma instância ouvindo nas portas usuais (`3456`–`3555`) e respondendo como Tokalytics, o comando apenas informa a URL e encerra.

| Flag / comando | Efeito |
|----------------|--------|
| *(nenhuma)* ou `--start` | Inicia menu bar + dashboard se não houver instância; caso contrário, mensagem em stdout. |
| `--status` | Mostra se há instância rodando, URL, versão da API (`/api/health`) e PID quando o `runstate` bate com a porta. |
| `--stop` | Encerra a instância em execução (HTTP local `POST /api/shutdown`, só loopback). |
| `--reload` | Dispara atualização de dados na instância ativa (`GET /api/refresh`). |
| `--version`, `-v`, `--v` | Imprime a versão **deste binário** e sai. |
| `-h`, `--help` ou `tokalytics help` | Lista opções e sai com código 0. |

Estado auxiliar (PID/porta) fica em `~/.config/tokalytics/runstate.json` enquanto o app está rodando; é removido ao sair. Se a porta padrão `3456` estiver ocupada por outro programa, o Tokalytics tenta a próxima livre; use a URL indicada no log ou em **Settings** no dashboard.

## Dashboard

Interface web em abas:

| Aba | Conteúdo |
|-----|----------|
| **Visão geral** | Hero, quotas (local / provedores), explicação de tokens, totais |
| **Gráficos** | Gráfico diário de tokens, distribuição por modelo |
| **Insights** | Recomendações automáticas com base no período |
| **Prompts** | Mensagens que mais consumiram tokens |
| **Sessões** | Lista pesquisável; clique em uma sessão para ver o detalhe (turns, custo por turno) |

O dashboard também detecta **skills/plugins/MCPs** instalados no `Codex` via `~/.codex` e exibe sessões locais gravadas em `~/.codex/sessions`.

Há atalhos para **atualizar dados** e **configurações** (cookies opcionais para APIs em nuvem).

### Aviso de nova versão

Em builds com versão **semver** (releases; não `dev`), o dashboard consulta a última release no GitHub e, se houver versão mais nova, mostra um **painel lateral** com o comando `npm install -g tokalytics` (copiar com um clique) e link para a release. O aviso pode ser fechado e não reaparece para a mesma versão alvo até limpar o `localStorage` do site. A resposta do servidor é cacheada por cerca de **1 hora**; use `GITHUB_TOKEN` no ambiente se precisar de limite maior na API do GitHub.

## Licença

MIT

## Créditos

Baseado em ideias de ferramentas como **claude-spend** e **CodexBar**, evoluído como Tokalytics.
