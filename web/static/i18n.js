(function (global) {
  'use strict';

  var STORAGE_KEY = 'tokalytics-lang';

  var MONTHS = {
    en: ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'],
    'pt-BR': ['jan', 'fev', 'mar', 'abr', 'mai', 'jun', 'jul', 'ago', 'set', 'out', 'nov', 'dez'],
  };

  var HOW_EN = '<p>Every time you send a message, Claude doesn\'t just read your latest message. It re-reads your <strong>entire conversation</strong> from the beginning. That\'s why tokens add up fast.</p>' +
    '<div class="how-tokens-example">' +
    '<div class="how-tokens-example-title">Example: A 3-message conversation</div>' +
    '<div class="how-tokens-step"><span class="step-label">Message 1</span>' +
    '<span class="step-desc">You send "Help me fix this bug." Claude reads your message fresh (<span style="color:var(--indigo)">input</span>) and saves the conversation to cache (<span style="color:var(--rose)">cache write</span>).</span>' +
    '<span class="step-tokens">~4K tokens</span></div>' +
    '<div class="how-tokens-step"><span class="step-label">Message 2</span>' +
    '<span class="step-desc">You reply "Try the other file." Claude reuses message 1 from cache (<span style="color:var(--amber)">cache read</span> — 10x cheaper) and only processes your new message fresh (<span style="color:var(--indigo)">input</span>). Then saves the updated conversation (<span style="color:var(--rose)">cache write</span>).</span>' +
    '<span class="step-tokens">~8K tokens</span></div>' +
    '<div class="how-tokens-step"><span class="step-label">Message 3</span>' +
    '<span class="step-desc">Claude reuses messages 1-2 from cache (<span style="color:var(--amber)">cache read</span>) and processes your new message fresh (<span style="color:var(--indigo)">input</span>). Claude\'s reply is <span style="color:var(--teal)">output</span>.</span>' +
    '<span class="step-tokens">~14K tokens</span></div>' +
    '<div class="how-tokens-total">Without caching, every message would be processed at full price. Cache reads save you up to 10x on repeated context.</div></div>' +
    '<div class="how-tokens-types">' +
    '<div class="how-tokens-type"><div class="how-tokens-dot" style="background:var(--indigo)"></div><div><strong>Input</strong> — Your message + conversation history, processed fresh. Full price.</div></div>' +
    '<div class="how-tokens-type"><div class="how-tokens-dot" style="background:var(--rose)"></div><div><strong>Cache writes</strong> — Claude saves your conversation so it doesn\'t re-process everything next time. Costs 1.25x input, but only happens once.</div></div>' +
    '<div class="how-tokens-type"><div class="how-tokens-dot" style="background:var(--amber)"></div><div><strong>Cache reads</strong> — Follow-up messages reuse the saved context. 10x cheaper than input. More cache reads = more savings.</div></div>' +
    '<div class="how-tokens-type"><div class="how-tokens-dot" style="background:var(--teal)"></div><div><strong>Output</strong> — What Claude writes back. Most expensive per token, but usually the smallest amount.</div></div></div>' +
    '<div class="how-tokens-tip"><strong>Tip:</strong> Use <code>/clear</code> when switching tasks. It resets the conversation so Claude stops re-reading old context you no longer need.</div>';

  var HOW_PT = '<p>Sempre que você envia uma mensagem, o Claude não lê só a última. Ele relê a <strong>conversa inteira</strong> desde o começo. Por isso os tokens acumulam rápido.</p>' +
    '<div class="how-tokens-example">' +
    '<div class="how-tokens-example-title">Exemplo: conversa com 3 mensagens</div>' +
    '<div class="how-tokens-step"><span class="step-label">Mensagem 1</span>' +
    '<span class="step-desc">Você envia "Me ajuda a corrigir esse bug." O Claude lê sua mensagem como <span style="color:var(--indigo)">input</span> e grava a conversa no cache (<span style="color:var(--rose)">cache write</span>).</span>' +
    '<span class="step-tokens">~4K tokens</span></div>' +
    '<div class="how-tokens-step"><span class="step-label">Mensagem 2</span>' +
    '<span class="step-desc">Você responde "Tenta o outro arquivo." O Claude reutiliza a mensagem 1 do cache (<span style="color:var(--amber)">cache read</span> — ~10x mais barato) e processa só o que é novo como <span style="color:var(--indigo)">input</span>. Depois salva de novo (<span style="color:var(--rose)">cache write</span>).</span>' +
    '<span class="step-tokens">~8K tokens</span></div>' +
    '<div class="how-tokens-step"><span class="step-label">Mensagem 3</span>' +
    '<span class="step-desc">O Claude reutiliza as mensagens 1–2 do cache (<span style="color:var(--amber)">cache read</span>) e processa a nova como <span style="color:var(--indigo)">input</span>. A resposta do Claude é <span style="color:var(--teal)">output</span>.</span>' +
    '<span class="step-tokens">~14K tokens</span></div>' +
    '<div class="how-tokens-total">Sem cache, cada mensagem custaria o preço cheio. Leituras de cache economizam até ~10x no contexto repetido.</div></div>' +
    '<div class="how-tokens-types">' +
    '<div class="how-tokens-type"><div class="how-tokens-dot" style="background:var(--indigo)"></div><div><strong>Input</strong> — Sua mensagem + histórico, processados do zero. Preço cheio.</div></div>' +
    '<div class="how-tokens-type"><div class="how-tokens-dot" style="background:var(--rose)"></div><div><strong>Cache writes</strong> — O Claude grava a conversa para não reprocessar tudo na próxima vez. Custa ~1,25x o input, mas só uma vez.</div></div>' +
    '<div class="how-tokens-type"><div class="how-tokens-dot" style="background:var(--amber)"></div><div><strong>Cache reads</strong> — Mensagens seguintes reutilizam o contexto salvo. ~10x mais barato que input. Mais leituras = mais economia.</div></div>' +
    '<div class="how-tokens-type"><div class="how-tokens-dot" style="background:var(--teal)"></div><div><strong>Output</strong> — O que o Claude devolve. Mais caro por token, em geral o menor volume.</div></div></div>' +
    '<div class="how-tokens-tip"><strong>Dica:</strong> use <code>/clear</code> ao mudar de tarefa. Zera a conversa e o Claude para de reler contexto que você não precisa mais.</div>';

  var M = {
    en: {
      howTokensTitle: 'How Claude Code tokens work',
      howTokensBodyHtml: HOW_EN,
      loading: 'Loading Tokalytics sessions…',
      hdrSettings: 'Settings',
      hdrRefresh: 'Refresh',
      hdrVersionTitle: 'Tokalytics version',
      hdrVersionAria: 'Tokalytics version',
      navAria: 'Dashboard sections',
      navOverview: 'Overview',
      navCharts: 'Charts',
      navInsights: 'Insights',
      navPrompts: 'Prompts',
      navSessions: 'Sessions',
      navResources: 'Resources',
      navPlugins: 'Plugins',
      footerNpm: 'npm package',
      footerLinkedIn: 'LinkedIn',
      langToggleAria: 'Language',
      resourcesLiveNote: 'CPU and RAM reflect your machine. An LLM model name appears only when the process command line exposes it; many rows are grouped as generic tool buckets.',
      drawerNoPromptData: 'No prompt data available',
      drawerTurnsExtra: '+ {n} turns',
      projectsCountOne: '1 project',
      projectsCount: '{n} projects',
      ariaModelsUsed: 'Models used: {list}',
      modelAriaTokenParts: '{inn} input, {out} output',
      projectRowInOut: '{inn} in · {out} out',
      liveToolMeta: 'Σ CPU {cpu}% · RAM {ram} · {n} proc.',
      liveOtherToolBucket: 'Other (tool)',
      fmtUnitB: ' B',
      fmtUnitKB: ' KB',
      fmtUnitMB: ' MB',
      fmtUnitGB: ' GB',
      chartDailyTitle: 'Tokens per day',
      chartDailyTip: 'How many tokens you used each day, broken down by type and cost.',
      legendInput: 'Input',
      legendCacheWrite: 'Cache writes',
      legendCacheRead: 'Cache reads',
      legendOutput: 'Output',
      chartByModel: 'By model',
      chartTotalCenter: 'total',
      chartMostExpensive: 'most expensive',
      insightsSection: 'Insights',
      insightsEmptyTitle: 'No insights',
      insightsEmptyBody: 'There are no recommendations for the selected period. Insights appear when usage patterns stand out.',
      promptsTitle: 'Most expensive prompts',
      promptsTip: 'Your top 20 individual messages ranked by how many tokens they used. Short vague messages like "Yes" often rank highest because they trigger long chains of tool calls where Claude tries to figure out what you meant.',
      sessionsTitle: 'All sessions',
      sessionsSearchPh: 'Search prompts, models, projects…',
      sessionsThDate: 'Date',
      sessionsThPrompt: 'What you asked',
      sessionsThModel: 'Model',
      sessionsThModelTip: 'Which AI model was used. Opus is the most capable (and uses more tokens), Sonnet is faster and lighter.',
      sessionsThMessages: 'Messages',
      sessionsThMessagesTip: 'How many back-and-forth messages happened in this conversation, including automatic tool calls Claude made.',
      sessionsThTotal: 'Total tokens',
      sessionsThTotalTip: 'The total number of tokens used in this conversation. Higher means more expensive. Long conversations cost more because Claude re-reads everything each turn.',
      sessionsThRead: 'Read',
      sessionsThReadTip: 'Tokens Claude read: your messages, the conversation history, files, and system context. This grows with each message because the full history gets re-sent.',
      sessionsThWritten: 'Written',
      sessionsThWrittenTip: 'Tokens Claude wrote back: responses, code, explanations. Usually a small fraction of the total.',
      pluginsTitle: 'Configured plugins',
      resourcesTitle: 'System resources',
      resourcesCpuHint: 'CPU per tool is the sum of detected processes; on multi-core machines this sum can exceed 100%. LLM models only appear when the name shows up in the process command line.',
      drillCostTitle: 'Cost per turn',
      drillCostSub: 'Each dot is one turn (your message + Claude\'s full response). Y-axis shows API-equivalent cost. You don\'t pay this on a subscription, but it shows how much work each turn requires.',
      settingsTitle: 'Settings — providers',
      settingsClaudeLabel: 'Claude — session cookie',
      settingsClaudeHint: 'Open claude.ai → DevTools (F12) → Application → Cookies → copy the value of',
      settingsCursorLabel: 'Cursor — session cookie',
      settingsCursorHint: 'Open cursor.com → DevTools → Application → Cookies → copy',
      settingsSystem: 'System',
      settingsAutostart: 'Start Tokalytics automatically when this user logs in',
      settingsSave: 'Save',
      settingsClear: 'Clear all',
      settingsSaving: 'Saving…',
      settingsSaved: 'Saved! Refreshing data…',
      settingsSaveErr: 'Could not save.',
      settingsPortDefault: 'Dashboard at http://localhost:3456 (default port).',
      settingsPortOther: 'Port 3456 was busy; this dashboard is at http://localhost:{port} .',
      settingsClaudeOk: 'Claude configured',
      settingsClaudeNo: 'Not configured',
      settingsCursorOk: 'Cursor configured',
      settingsCursorNo: 'Not configured',
      settingsAutostartNo: 'Autostart is not available on this operating system.',
      themeSystem: 'Match system',
      themeLight: 'Light',
      themeDark: 'Dark',
      themeBtnTitle: 'Theme: {label} — click to cycle',
      themeBtnAria: 'Theme: {label}. Click to cycle.',
      updateDismissAria: 'Dismiss update notice',
      updateTitle: 'New version on GitHub',
      updateIntroPrefix: 'Version ',
      updateIntroMid: ' is available. You are on ',
      updateIntroSuffix: '.',
      updateNpmLabel: 'Update (npm)',
      updateCopy: 'Copy command',
      updateLink: 'Open release page',
      updateCopied: 'Copied!',
      updateCopyManual: 'Select and copy',
      warningsHeadsUp: 'Heads up:',
      errTitle: 'Something went wrong',
      errRetry: 'Retry',
      errHintClaude: 'Make sure you have used Claude Code at least once. The data directory is created automatically.',
      errHintPerm: 'Try running with elevated permissions, or check file permissions on your ~/.claude directory.',
      errNoData: 'No session data found. Make sure you have used Claude Code at least once.',
      errLoadPrefix: 'Failed to load data: ',
      errServerPrefix: 'Server returned ',
      statTotalUsage: 'Total usage',
      statTotalUsageTip: 'All tokens across your conversations. Input = fresh context, cache writes = first-time caching, cache reads = reused from cache (10x cheaper), output = Claude\'s responses.',
      statConversations: 'Conversations',
      statConversationsTip: 'Each time you start Claude Code and begin chatting, that counts as one conversation. A new conversation starts fresh with no prior context.',
      statMessages: 'Messages sent',
      statMessagesTip: 'Every time you hit Enter and send something to Claude, that is one message. This includes follow-up tool calls Claude makes automatically behind the scenes.',
      statCacheHit: 'Cache hit rate',
      statCacheHitTip: 'The percentage of input tokens that were served from cache instead of being processed fresh. Higher is better — cached tokens are 10x cheaper and help you stay under rate limits.',
      statSubAvgSession: 'Each used ~{n} tokens on average',
      statSubAvgMsg: 'Each message used ~{n} tokens on average',
      statSubCache: '{n} tokens served from cache',
      statSubTokensBreakdown: '{input} input · {cw} cache writes · {cr} cache reads · {out} output',
      providerLocalToday: 'Local · today',
      providerLocal: 'Local',
      providerNoSessionsToday: 'No sessions found today.',
      providerUseClaude: 'Use Claude Code to generate data.',
      providerEstimated: '${cost} estimated · {sessions} {sessWord}',
      providerSession: 'session',
      providerSessions: 'sessions',
      providerInOut: '{inn} in · {cached} cached · {out} out',
      windowPctLeft: '{n}% left',
      windowResetsIn: 'Resets in {s}',
      durMin: '{n}m',
      durHours: '{n}h',
      durHoursMin: '{n}h {m}m',
      durDays: '{n}d',
      durDaysHours: '{n}d {h}h',
      windowDeficit: '{n}% over limit',
      costHeading: 'Cost',
      costToday: 'Today: {usd} · {tok} tokens',
      cost30d: '30 days: {usd} · {tok} tokens',
      updatedAt: 'Updated {at}',
      liveLabelCpu: 'CPU (machine)',
      liveSubCpu: 'average across all cores',
      liveLabelMem: 'Memory in use',
      liveErr: 'Error',
      liveErrRead: 'Could not read system state',
      liveNoProc: 'no processes',
      liveNoProcDetail: 'No processes for this tool are running right now.',
      liveThModel: 'Model / group',
      liveThCpu: 'CPU',
      liveThRam: 'RAM',
      liveThProc: 'Proc.',
      pluginsSectionPlugins: 'Plugins',
      pluginsSectionMcps: 'MCPs',
      pluginsSectionSkills: 'Skills',
      pluginsNoneDetected: 'None detected',
      pluginsNoneItems: 'No items detected',
      pluginsLoadErr: 'Could not load plugins',
      topPromptTok: '{inn} in / {cached} cached / {out} out',
      sessionsCount: '{n} sessions',
      sessionsCountOne: '1 session',
      drillMeta: '{date} · {model} · {n} messages · {tok} tokens',
      drillToolUses: ' + {n} tool uses',
      insightSpikeTitle: 'Turn {turn} cost ${cost} — {ratio}x more than a typical turn',
      insightSpikeTitleFar: 'Turn {turn} cost ${cost} — far more than a typical turn',
      insightSpikeBody: 'Median turn cost: ${median}. The spike at turn {turn} happened because context had grown large. Running /clear before expensive turns resets context and brings costs back down. Tip: if Claude slows down or you switch tasks, that is a good time to /clear.',
      insightLongTitle: '{n} turns is a long conversation',
      insightLongBody: 'Caching kept costs steady (avg ~${avg}/turn), but the context window still grew large. Late turns averaged ~${late}. Consider splitting into smaller conversations for better focus and lower risk of hitting context limits.',
      insightOkTitle: 'This conversation was efficient',
      insightOkBody: '{n} turns, avg ~${avg}/turn, total ~${total} API-equivalent. {cacheNote} No issues detected.',
      insightCacheGood: 'Cache hit rate: {p}% (good).',
      insightCacheSome: 'Cache hit rate: {p}%.',
    },
    'pt-BR': {
      howTokensTitle: 'Como funcionam os tokens do Claude Code',
      howTokensBodyHtml: HOW_PT,
      loading: 'Carregando sessões do Tokalytics…',
      hdrSettings: 'Configurações',
      hdrRefresh: 'Atualizar',
      hdrVersionTitle: 'Versão do Tokalytics',
      hdrVersionAria: 'Versão do Tokalytics',
      navAria: 'Seções do dashboard',
      navOverview: 'Visão geral',
      navCharts: 'Gráficos',
      navInsights: 'Insights',
      navPrompts: 'Prompts',
      navSessions: 'Sessões',
      navResources: 'Recursos',
      navPlugins: 'Plugins',
      footerNpm: 'Pacote npm',
      footerLinkedIn: 'LinkedIn',
      langToggleAria: 'Idioma',
      resourcesLiveNote: 'CPU e RAM refletem sua máquina. O nome do modelo de LLM só aparece quando a linha de comando do processo o expõe; muitas linhas são agrupadas como buckets genéricos da ferramenta.',
      drawerNoPromptData: 'Nenhum dado de prompt disponível',
      drawerTurnsExtra: '+ {n} turnos',
      projectsCountOne: '1 projeto',
      projectsCount: '{n} projetos',
      ariaModelsUsed: 'Modelos usados: {list}',
      modelAriaTokenParts: '{inn} entrada, {out} saída',
      projectRowInOut: '{inn} entrada · {out} saída',
      liveToolMeta: 'Σ CPU {cpu}% · RAM {ram} · {n} proc.',
      liveOtherToolBucket: 'Outros (ferramenta)',
      fmtUnitB: ' B',
      fmtUnitKB: ' KB',
      fmtUnitMB: ' MB',
      fmtUnitGB: ' GB',
      chartDailyTitle: 'Tokens por dia',
      chartDailyTip: 'Quantos tokens você usou por dia, por tipo e custo.',
      legendInput: 'Entrada',
      legendCacheWrite: 'Gravações no cache',
      legendCacheRead: 'Leituras do cache',
      legendOutput: 'Saída',
      chartByModel: 'Por modelo',
      chartTotalCenter: 'total',
      chartMostExpensive: 'mais cara',
      insightsSection: 'Insights',
      insightsEmptyTitle: 'Nenhum insight',
      insightsEmptyBody: 'Não há recomendações para o período selecionado. Insights aparecem quando há padrões relevantes no uso.',
      promptsTitle: 'Prompts mais caros',
      promptsTip: 'Suas 20 mensagens individuais que mais gastaram tokens. Mensagens curtas e vagas como "Sim" costumam aparecer no topo porque disparam longas cadeias de tool calls enquanto o Claude tenta entender o que você quis dizer.',
      sessionsTitle: 'Todas as sessões',
      sessionsSearchPh: 'Buscar prompts, modelos, projetos…',
      sessionsThDate: 'Data',
      sessionsThPrompt: 'O que você pediu',
      sessionsThModel: 'Modelo',
      sessionsThModelTip: 'Qual modelo foi usado. Opus é o mais capaz (e usa mais tokens), Sonnet é mais rápido e leve.',
      sessionsThMessages: 'Mensagens',
      sessionsThMessagesTip: 'Quantas idas e vindas na conversa, incluindo tool calls automáticas do Claude.',
      sessionsThTotal: 'Total de tokens',
      sessionsThTotalTip: 'Total de tokens nesta conversa. Valores maiores costumam ser mais caros. Conversas longas custam mais porque o Claude relê tudo a cada turno.',
      sessionsThRead: 'Lido',
      sessionsThReadTip: 'Tokens que o Claude leu: suas mensagens, histórico, arquivos e contexto do sistema. Cresce a cada mensagem porque o histórico completo é reenviado.',
      sessionsThWritten: 'Escrito',
      sessionsThWrittenTip: 'Tokens que o Claude escreveu: respostas, código, explicações. Em geral uma fração pequena do total.',
      pluginsTitle: 'Plugins configurados',
      resourcesTitle: 'Recursos do sistema',
      resourcesCpuHint: 'CPU por ferramenta é a soma dos processos detectados; em máquinas com vários núcleos essa soma pode passar de 100%. Modelos de LLM só aparecem quando o nome aparece na linha de comando do processo.',
      drillCostTitle: 'Custo por turno',
      drillCostSub: 'Cada ponto é um turno (sua mensagem + resposta completa do Claude). O eixo Y é custo equivalente à API. Em assinatura você não paga isso diretamente, mas mostra o esforço de cada turno.',
      settingsTitle: 'Configurações — provedores',
      settingsClaudeLabel: 'Claude — cookie de sessão',
      settingsClaudeHint: 'Abra claude.ai → DevTools (F12) → Application → Cookies → copie o valor de',
      settingsCursorLabel: 'Cursor — cookie de sessão',
      settingsCursorHint: 'Abra cursor.com → DevTools → Application → Cookies → copie',
      settingsSystem: 'Sistema',
      settingsAutostart: 'Iniciar o Tokalytics automaticamente ao fazer login neste usuário',
      settingsSave: 'Salvar',
      settingsClear: 'Limpar tudo',
      settingsSaving: 'Salvando…',
      settingsSaved: 'Salvo! Atualizando dados…',
      settingsSaveErr: 'Erro ao salvar.',
      settingsPortDefault: 'Dashboard em http://localhost:3456 (porta padrão).',
      settingsPortOther: 'A porta 3456 estava ocupada; este painel está em http://localhost:{port} .',
      settingsClaudeOk: 'Claude configurado',
      settingsClaudeNo: 'Não configurado',
      settingsCursorOk: 'Cursor configurado',
      settingsCursorNo: 'Não configurado',
      settingsAutostartNo: 'Início automático não está disponível neste sistema operacional.',
      themeSystem: 'Automático (sistema)',
      themeLight: 'Claro',
      themeDark: 'Escuro',
      themeBtnTitle: 'Tema: {label} — clique para alternar',
      themeBtnAria: 'Tema: {label}. Clique para alternar.',
      updateDismissAria: 'Fechar aviso de atualização',
      updateTitle: 'Nova versão no GitHub',
      updateIntroPrefix: 'A versão ',
      updateIntroMid: ' está disponível. Você está na ',
      updateIntroSuffix: '.',
      updateNpmLabel: 'Atualizar (npm)',
      updateCopy: 'Copiar comando',
      updateLink: 'Abrir página da release',
      updateCopied: 'Copiado!',
      updateCopyManual: 'Selecione e copie',
      warningsHeadsUp: 'Atenção:',
      errTitle: 'Algo deu errado',
      errRetry: 'Tentar de novo',
      errHintClaude: 'Use o Claude Code pelo menos uma vez. O diretório de dados é criado automaticamente.',
      errHintPerm: 'Tente executar com permissões elevadas ou verifique as permissões em ~/.claude.',
      errNoData: 'Nenhuma sessão encontrada. Use o Claude Code pelo menos uma vez.',
      errLoadPrefix: 'Falha ao carregar dados: ',
      errServerPrefix: 'Servidor retornou ',
      statTotalUsage: 'Uso total',
      statTotalUsageTip: 'Todos os tokens nas conversas. Input = contexto novo, cache writes = primeira gravação no cache, cache reads = reutilização do cache (~10x mais barato), output = respostas do Claude.',
      statConversations: 'Conversas',
      statConversationsTip: 'Cada vez que você abre o Claude Code e começa a conversar conta como uma conversa. Uma conversa nova começa sem contexto anterior.',
      statMessages: 'Mensagens enviadas',
      statMessagesTip: 'Cada vez que você pressiona Enter e envia algo ao Claude conta como uma mensagem, incluindo tool calls automáticas.',
      statCacheHit: 'Taxa de acerto do cache',
      statCacheHitTip: 'Percentual de tokens de entrada servidos pelo cache em vez de processados do zero. Quanto maior, melhor — tokens em cache são ~10x mais baratos e ajudam nos limites.',
      statSubAvgSession: 'Cada uma usou ~{n} tokens em média',
      statSubAvgMsg: 'Cada mensagem usou ~{n} tokens em média',
      statSubCache: '{n} tokens vindos do cache',
      statSubTokensBreakdown: '{input} entrada · {cw} gravações no cache · {cr} leituras do cache · {out} saída',
      providerLocalToday: 'Local · hoje',
      providerLocal: 'Local',
      providerNoSessionsToday: 'Nenhuma sessão encontrada hoje.',
      providerUseClaude: 'Use o Claude Code para gerar dados.',
      providerEstimated: '${cost} estimado · {sessions} {sessWord}',
      providerSession: 'sessão',
      providerSessions: 'sessões',
      providerInOut: '{inn} entrada · {cached} em cache · {out} saída',
      windowPctLeft: '{n}% restantes',
      windowResetsIn: 'Renova em {s}',
      durMin: '{n} min',
      durHours: '{n} h',
      durHoursMin: '{n} h {m} min',
      durDays: '{n} d',
      durDaysHours: '{n} d {h} h',
      windowDeficit: '{n}% acima do limite',
      costHeading: 'Custo',
      costToday: 'Hoje: {usd} · {tok} tokens',
      cost30d: '30 dias: {usd} · {tok} tokens',
      updatedAt: 'Atualizado {at}',
      liveLabelCpu: 'CPU (máquina)',
      liveSubCpu: 'média de todos os núcleos',
      liveLabelMem: 'Memória em uso',
      liveErr: 'Erro',
      liveErrRead: 'Falha ao ler o sistema',
      liveNoProc: 'nenhum processo',
      liveNoProcDetail: 'Nenhum processo desta ferramenta em execução no momento.',
      liveThModel: 'Modelo / grupo',
      liveThCpu: 'CPU',
      liveThRam: 'RAM',
      liveThProc: 'Proc.',
      pluginsSectionPlugins: 'Plugins',
      pluginsSectionMcps: 'MCPs',
      pluginsSectionSkills: 'Habilidades',
      pluginsNoneDetected: 'Nenhum detectado',
      pluginsNoneItems: 'Nenhum item detectado',
      pluginsLoadErr: 'Erro ao carregar plugins',
      topPromptTok: '{inn} entrada / {cached} em cache / {out} saída',
      sessionsCount: '{n} sessões',
      sessionsCountOne: '1 sessão',
      drillMeta: '{date} · {model} · {n} mensagens · {tok} tokens',
      drillToolUses: ' + {n} usos de ferramenta',
      insightSpikeTitle: 'Turno {turn} custou ${cost} — {ratio}x mais que um turno típico',
      insightSpikeTitleFar: 'Turno {turn} custou ${cost} — muito mais que um turno típico',
      insightSpikeBody: 'Custo mediano por turno: ${median}. O pico no turno {turn} ocorreu porque o contexto já estava grande. Usar /clear antes de turnos caros zera o contexto e reduz o custo. Dica: se o Claude ficar lento ou você mudar de tarefa, é um bom momento para /clear.',
      insightLongTitle: '{n} turnos é uma conversa longa',
      insightLongBody: 'O cache manteve os custos estáveis (média ~${avg}/turno), mas a janela de contexto ainda cresceu. Turnos finais tiveram média ~${late}. Considere dividir em conversas menores para foco e menos risco de estourar o limite.',
      insightOkTitle: 'Esta conversa foi eficiente',
      insightOkBody: '{n} turnos, média ~${avg}/turno, total ~${total} (equivalente API). {cacheNote} Nada crítico detectado.',
      insightCacheGood: 'Taxa de acerto do cache: {p}% (boa).',
      insightCacheSome: 'Taxa de acerto do cache: {p}%.',
    },
  };

  function detectLang() {
    try {
      var v = localStorage.getItem(STORAGE_KEY);
      if (v === 'en' || v === 'pt-BR') return v;
    } catch (e) {}
    var nav = (navigator.language || '').toLowerCase();
    return nav.indexOf('pt') === 0 ? 'pt-BR' : 'en';
  }

  var current = detectLang();

  function setDocumentLang() {
    document.documentElement.lang = current === 'pt-BR' ? 'pt-BR' : 'en';
  }

  function interpolate(str, vars) {
    if (!vars) return str;
    var s = str;
    Object.keys(vars).forEach(function (k) {
      s = s.split('{' + k + '}').join(String(vars[k]));
    });
    return s;
  }

  function t(key, vars) {
    var table = M[current] || M.en;
    var raw = table[key];
    if (raw == null) raw = M.en[key];
    if (raw == null) return key;
    return interpolate(raw, vars);
  }

  function applyUiI18n() {
    document.querySelectorAll('[data-i18n]').forEach(function (el) {
      var k = el.getAttribute('data-i18n');
      if (!k) return;
      if (el.getAttribute('data-i18n-html') === '1') {
        el.innerHTML = t(k);
      } else {
        el.textContent = t(k);
      }
    });
    document.querySelectorAll('[data-i18n-placeholder]').forEach(function (el) {
      var k = el.getAttribute('data-i18n-placeholder');
      if (k) el.setAttribute('placeholder', t(k));
    });
    document.querySelectorAll('[data-i18n-title]').forEach(function (el) {
      var k = el.getAttribute('data-i18n-title');
      if (k) el.title = t(k);
    });
    document.querySelectorAll('[data-i18n-aria]').forEach(function (el) {
      var k = el.getAttribute('data-i18n-aria');
      if (k) el.setAttribute('aria-label', t(k));
    });
    var howBody = document.getElementById('howTokensBody');
    if (howBody) howBody.innerHTML = t('howTokensBodyHtml');
    var upPre = document.getElementById('updateSlideIntroPrefix');
    var upMid = document.getElementById('updateSlideIntroMid');
    var upSuf = document.getElementById('updateSlideIntroSuffix');
    if (upPre) upPre.textContent = t('updateIntroPrefix');
    if (upMid) upMid.textContent = t('updateIntroMid');
    if (upSuf) upSuf.textContent = t('updateIntroSuffix');
    if (typeof global.syncThemeToggleUI === 'function') global.syncThemeToggleUI();
    if (typeof global.syncLangToggleUI === 'function') global.syncLangToggleUI();
  }

  function formatDayMonth(dashDate) {
    if (!dashDate) return '';
    var parts = dashDate.split('-');
    if (parts.length < 3) return '';
    var months = MONTHS[current] || MONTHS.en;
    var mi = parseInt(parts[1], 10) - 1;
    if (mi < 0 || mi > 11) return '';
    return months[mi] + ' ' + parseInt(parts[2], 10);
  }

  function setLang(lang) {
    if (lang !== 'en' && lang !== 'pt-BR') return;
    current = lang;
    try {
      localStorage.setItem(STORAGE_KEY, lang);
    } catch (e) {}
    setDocumentLang();
    applyUiI18n();
    if (typeof global.__tokalyticsRerender === 'function') global.__tokalyticsRerender();
  }

  function getLang() {
    return current;
  }

  function cycleLang() {
    setLang(current === 'en' ? 'pt-BR' : 'en');
  }

  setDocumentLang();

  global.TokalyticsI18n = {
    t: t,
    getLang: getLang,
    setLang: setLang,
    cycleLang: cycleLang,
    apply: applyUiI18n,
    formatDayMonth: formatDayMonth,
    fmtResetCountdown: function (date) {
      var secs = Math.max(0, (date - new Date()) / 1000);
      var m = Math.ceil(secs / 60);
      if (m < 60) return t('durMin', { n: String(m) });
      var h = Math.floor(m / 60);
      var rem = m % 60;
      if (h < 24) return rem > 0 ? t('durHoursMin', { n: String(h), m: String(rem) }) : t('durHours', { n: String(h) });
      var d = Math.floor(h / 24);
      var rh = h % 24;
      return rh > 0 ? t('durDaysHours', { n: String(d), h: String(rh) }) : t('durDays', { n: String(d) });
    },
  };

  global.t = t;
})(typeof window !== 'undefined' ? window : this);
