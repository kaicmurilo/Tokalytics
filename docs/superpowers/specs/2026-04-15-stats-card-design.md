# Stats Card Unificado — Design Spec
**Data:** 2026-04-15
**Status:** Aprovado

---

## Visão Geral

Substituir a `statsRow` existente na página Overview por um card unificado com duas abas (Overview / Models), filtros de tempo (All / 30d / 7d), heatmap de atividade GitHub-style e comparações divertidas baseadas em tokens totais.

Nenhum endpoint de API novo é necessário — todos os cálculos são client-side a partir dos dados já disponíveis em `/api/data`.

---

## Arquitetura

### Arquivos modificados
- `web/static/index.html` — estrutura HTML, CSS e JS do novo componente
- `web/static/i18n.js` — strings novas em `en` e `pt-BR`

### Sem mudanças no backend
Todos os campos necessários já existem:
- `DATA.sessions[].timestamp` → Peak hour
- `DATA.dailyUsage[]` → Active days, streaks, heatmap
- `DATA.modelBreakdown[]` → Favorite model, aba Models
- `DATA.totals` → Sessions, Messages, Total tokens

---

## Componente: Stats Card

### HTML Structure
```
#statsCard
  .stats-card-header
    .stats-tabs         (Overview | Models)
    .stats-filters      (All | 30d | 7d)
  #statsTabOverview
    .stats-grid         (2×4 metric cards)
    #statsHeatmap       (canvas ou divs)
    #statsComparison    (texto divertido)
  #statsTabModels
    #statsModelsList    (cards por modelo)
```

### Filtros de Tempo
- `All` — todos os dados
- `30d` — últimos 30 dias
- `7d` — últimos 7 dias

Ao trocar filtro: refiltra `dailyUsage` e `sessions` por data, recalcula todas as métricas, re-renderiza heatmap, métricas e comparação.

Estado do filtro ativo salvo em variável JS local (não persistido).

---

## Aba Overview

### Grid de Métricas (2×4)

| Posição | Label | Cálculo |
|---------|-------|---------|
| 1 | Sessions | `filteredTotals.totalSessions` |
| 2 | Messages | `filteredTotals.totalQueries` |
| 3 | Total tokens | `filteredTotals.totalTokens` |
| 4 | Active days | dias em `filteredDailyUsage` com `totalTokens > 0` |
| 5 | Current streak | dias consecutivos até hoje com atividade |
| 6 | Longest streak | maior sequência contínua no histórico filtrado |
| 7 | Peak hour | hora mais frequente em `filteredSessions[].timestamp` |
| 8 | Favorite model | modelo com mais tokens em `filteredModelBreakdown` |

**Current streak:** percorre dias de hoje para trás enquanto `dailyUsage[date].totalTokens > 0`.
**Longest streak:** varre `dailyUsage` em ordem cronológica, rastreia sequência atual e máximo.
**Peak hour:** parseia `session.timestamp` → `new Date().getHours()`, agrupa por hora, retorna a mais frequente formatada como "12 PM".
**Favorite model:** ordena `modelBreakdown` por `totalTokens` desc, retorna `[0].model`.

### Heatmap

- Grade de 52 semanas × 7 dias (domingo a sábado)
- Cada célula = 1 dia; cor proporcional a `totalTokens` daquele dia
- Escala de cor: 5 níveis (0 = cinza claro, 4 = indigo escuro)
- Tooltip ao hover: `"2026-03-15 · 142k tokens"`
- Responsive: em mobile mostra últimas 26 semanas
- Suporta dark mode via variáveis CSS existentes

### Comparação Divertida

Pool de ~20 templates, 1 escolhido aleatoriamente ao renderizar. Calcula multiplicador baseado em `totalTokens`.

Categorias (mistura):
- **Livros:** Harry Potter série (~500k tokens), Dom Quixote (~430k), Senhor dos Anéis (~500k), A Bíblia (~780k)
- **Filmes/Scripts:** Star Wars roteiro (~15k), Pulp Fiction roteiro (~14k)
- **Mensagens:** WhatsApp médio (~15 tokens), tweet (~30 tokens), email (~200 tokens)
- **Web:** artigo Wikipedia médio (~900 tokens), artigo de jornal (~500 tokens)

Exemplo: `totalTokens = 20_600_000` → "Você usou tokens suficientes para ler a série Harry Potter completa **41×**"

Formato: texto em itálico, centralizado, abaixo do heatmap.

---

## Aba Models

Lista vertical de cards, um por modelo em `modelBreakdown`, ordenados por `totalTokens` desc.

### Cada Model Card contém:
- Nome do modelo (badge estilizado, igual ao existente)
- Barra de progresso relativa ao modelo com mais tokens
- Total tokens (formatado: 20.6M)
- Custo ($0.00)
- Sessões e mensagens
- % do uso total

Responde ao filtro de tempo (refiltra sessions por modelo e data).

---

## i18n

Novas chaves a adicionar em `web/static/i18n.js`:

```
statsCardOverview, statsCardModels
statsFilterAll, statsFilter30d, statsFilter7d
statActiveDays, statCurrentStreak, statLongestStreak
statPeakHour, statFavoriteModel
statStreakDays (ex: "{n}d")
statPeakHourFmt (ex: "{h} PM" / "{h} AM")
heatmapTooltip (ex: "{date} · {tokens} tokens")
heatmapNoActivity
comparisonPrefix (ex: "Você usou tokens suficientes para")
modelCardSessions, modelCardMessages, modelCardOfTotal
```

---

## Comportamento de Filtro

Função `applyStatsFilter(period)`:
1. Filtra `DATA.dailyUsage` por data >= today - N dias
2. Filtra `DATA.sessions` por `session.date` >= corte
3. Recalcula `filteredTotals` somando tokens/custo/sessões/queries das sessions filtradas
4. Recalcula `filteredModelBreakdown` das sessions filtradas
5. Re-renderiza métricas, heatmap, comparação e aba Models

---

## CSS / Estilo

- Card com `border-radius: var(--radius)`, `box-shadow: var(--shadow-md)`, `background: var(--white)`
- Tabs estilo pill (fundo `--indigo` na aba ativa)
- Filtros estilo botão pequeno, borda sutil, ativo com fundo `--surface-hover-strong`
- Métrica card: label pequeno `--text-tertiary`, valor grande bold, sem borda externa (grid interno)
- Heatmap: células `6×6px` com `gap: 2px`, `border-radius: 2px`
- Compatível com dark mode via variáveis CSS já definidas

---

## Fora do Escopo

- Novos endpoints de API
- Persistência do filtro selecionado
- Animações de transição entre abas (só display show/hide)
- Exportação de dados
