#!/usr/bin/env node
'use strict';

const { spawnSync } = require('child_process');
const path = require('path');

const root = path.join(__dirname, '..');
const goBin = process.env.TOKALYTICS_GO || 'go';
const args = process.argv.slice(2);

if (args.length === 0) {
  console.error('Tokalytics: passe argumentos do go (ex.: build -o tokalytics .)');
  process.exit(1);
}

const opts = {
  cwd: root,
  stdio: 'inherit',
  env: process.env,
  windowsHide: true,
};

const r = spawnSync(goBin, args, opts);

if (!r.error && r.status === 0) {
  process.exit(0);
}

// macOS/Linux: fallback para o bootstrap via bash (Go baixado em ~/.local/share/tokalytics-go)
if (r.error && r.error.code === 'ENOENT' && process.platform !== 'win32') {
  const sh = path.join(__dirname, 'with-go.sh');
  const r2 = spawnSync('bash', [sh, ...args], opts);
  if (!r2.error && r2.status === 0) {
    process.exit(0);
  }
  if (r2.error) {
    console.error('Tokalytics: "go" não encontrado e falha ao executar bash', sh, '-', r2.error.message);
  } else {
    process.exit(typeof r2.status === 'number' ? r2.status : 1);
  }
}

if (r.error) {
  console.error('Tokalytics:', r.error.message);
  console.error(
    'Instale Go e garanta que "go" está no PATH: https://go.dev/dl/ (Windows: defina TOKALYTICS_GO se o executável não for "go")'
  );
  process.exit(1);
}

process.exit(typeof r.status === 'number' ? r.status : 1);
