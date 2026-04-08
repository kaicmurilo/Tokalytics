# Tokalytics

Ferramenta **local** (Go) que agrega uso de tokens e quotas: **menu bar** (system tray) + **dashboard web** para Claude Code, Cursor, Gemini CLI e dados em nuvem (Claude), quando configurado.

Os dados permanecem na sua máquina; nada é enviado para servidores externos pelo app.

> **Se `npm install -g tokalytics` falha com** `Cannot read properties of undefined (reading 'find')`: o npm ainda está servindo **`tokalytics@2.0.0`** (postinstall antigo). Confira com `npm view tokalytics version`. **Correção:** publique a versão nova (abaixo) ou instale pelo Git: `npm install -g "github:kaicmurilo/Tokalytics"`.

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

Confira a versão no registry: `npm view tokalytics version`. Se estiver antiga (por exemplo só `2.0.0`), `npm install -g tokalytics` usa um `postinstall` desatualizado.

- Depois de publicar a versão nova: `npm install -g tokalytics@latest` (ou `@2.0.3`).
- **Sem esperar o registry**, instalando o pacote a partir do GitHub (usa o `postinstall` da `main`):

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

## Release (mantenedores)

Com `NPM_TOKEN` configurado em **GitHub → Settings → Secrets** (token granular com permissão de publish):

1. Commit e push das mudanças em `main`.
2. Crie e envie a tag: `git tag vX.Y.Z && git push origin vX.Y.Z`.

O workflow [`.github/workflows/release.yml`](.github/workflows/release.yml) gera os binários na release do GitHub (nomes esperados pelo `npm install`) e publica o pacote no npm. Se o job **Publish to npm** falhar (token/2FA), o registry fica desatualizado mesmo com releases no GitHub.

**Publicação manual (conta com 2FA):** na raiz do repo, com `npm whoami` ok:

```bash
npm run release:npm -- --otp=CODIGO_DO_AUTENTICADOR
```

(Equivalente a `npm publish --access public --otp=...`.)

## Licença

MIT

## Créditos

Baseado em ideias de ferramentas como **claude-spend** e **CodexBar**, evoluído como Tokalytics.
