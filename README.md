# Tokalytics

Ferramenta **local** (Go) que agrega uso de tokens e quotas: **menu bar** (system tray) + **dashboard web** para Claude Code, Cursor, Gemini CLI e dados em nuvem (Claude), quando configurado.

Os dados permanecem na sua máquina; nada é enviado para servidores externos pelo app.

## Requisitos

- [Go](https://go.dev/dl/) 1.21+ (para build / `go run`)
- Node/npm opcional — apenas atalhos em `package.json` para os mesmos comandos Go

## Como executar

### Desenvolvimento (sem gerar binário)

```bash
go run main.go
```

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
npm run dev      # go run main.go
npm run build    # go build -o tokalytics main.go
npm run start    # compila e em seguida executa ./tokalytics
```

Na primeira execução o app sobe o **servidor HTTP na porta `3456`** e o ícone na barra de menus. Use **«Abrir Dashboard»** no menu ou acesse [http://127.0.0.1:3456](http://127.0.0.1:3456).

### Instalação global pelo npm

```bash
npm install -g tokalytics
```

Instalação a partir do repositório (sempre o `postinstall` da branch atual):

```bash
npm install -g "github:kaicmurilo/Tokalytics"
```

## Dashboard

Interface web em abas:

| Aba | Conteúdo |
|-----|----------|
| **Visão geral** | Hero, quotas (local / provedores), explicação de tokens, totais |
| **Gráficos** | Gráfico diário de tokens, distribuição por modelo |
| **Insights** | Recomendações automáticas com base no período |
| **Prompts** | Mensagens que mais consumiram tokens |
| **Sessões** | Lista pesquisável; clique em uma sessão para ver o detalhe (turns, custo por turno) |

Há atalhos para **atualizar dados**, **compartilhar stats** (PNG) e **configurações** (cookies opcionais para APIs em nuvem).

## Licença

MIT

## Créditos

Baseado em ideias de ferramentas como **claude-spend** e **CodexBar**, evoluído como Tokalytics.
