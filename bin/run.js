#!/usr/bin/env node
'use strict';

const path = require('path');
const { spawnSync } = require('child_process');
const fs = require('fs');

const BIN_PATH = path.join(__dirname, process.platform === 'win32' ? 'tokalytics.exe' : 'tokalytics');

if (!fs.existsSync(BIN_PATH)) {
  console.error('Tokalytics: binário não encontrado. Tente reinstalar: npm install -g tokalytics');
  process.exit(1);
}

const result = spawnSync(BIN_PATH, process.argv.slice(2), { stdio: 'inherit' });

if (result.error) {
  console.error('Erro ao executar tokalytics:', result.error.message);
  process.exit(1);
}

process.exit(result.status ?? 0);
