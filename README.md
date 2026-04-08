# Tokalytics (CodexBar + Claude Spend)

**One-liner:** Um System Tray App nativo e Dashboard Web para uso de tokens e quotas de APIs (Claude, Cursor, Gemini, Codex, Copilot, etc), re-escrito em Go para máxima performance.

## Funcionalidades
- **Menu Bar App**: Acesso rápido a quotas e custos direto da barra de tarefas do sistema.
- **Notificações**: Alertas nativos do SO quando as quotas esgotam.
- **Multi-Provedor**: Arquitetura pronta para suportar até 15+ provedores de IA integrados.

## Instalação e Execução

```bash
go run main.go
```

Isso inicializa:
1. O ícone na sua barra de menus.
2. O Dashboard web na porta `3456`.

## Licença
MIT
