# Tokalytics

Ferramenta **local** (Go) que agrega uso de tokens e quotas: **menu bar** (system tray) + **dashboard web** para Claude Code, Cursor, Gemini CLI e dados em nuvem (Claude), quando configurado.

Os dados permanecem na sua máquina; nada é enviado para servidores externos pelo app.

## Requisitos

- [Go](https://go.dev/dl/) 1.21+ (para build / `go run`)
- Node/npm opcional — apenas atalhos em `package.json` para os mesmos comandos Go

## Como executar

```bash
go run main.go
```

Ou, com npm:

```bash
npm run dev      # go run main.go
npm run start    # go build -o tokalytics && ./tokalytics
npm run build    # só compila o binário tokalytics
```

Na primeira execução o app sobe o **servidor HTTP na porta `3456`** e o ícone na barra de menus. Use **«Abrir Dashboard»** no menu ou acesse [http://127.0.0.1:3456](http://127.0.0.1:3456).

## Dashboard

Interface web em abas:

| Aba | Conteúdo |
|-----|----------|
| **Visão geral** | Hero, quotas (local / provedores), explicação de tokens, totais |
| **Gráficos e insights** | Insights automáticos, gráfico diário, distribuição por modelo |
| **Prompts** | Mensagens que mais consumiram tokens |
| **Sessões** | Lista pesquisável; clique em uma sessão para ver o detalhe (turns, custo por turno) |

Há atalhos para **atualizar dados**, **compartilhar stats** (PNG) e **configurações** (cookies opcionais para APIs em nuvem).

## Licença

MIT

## Créditos

Baseado em ideias de ferramentas como **claude-spend** e **CodexBar**, evoluído como Tokalytics.
