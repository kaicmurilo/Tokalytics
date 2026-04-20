#!/usr/bin/env node
'use strict';

const path = require('path');
const { spawnSync } = require('child_process');

const root = path.join(__dirname, '..');
const bin = path.join(root, process.platform === 'win32' ? 'tokalytics.exe' : 'tokalytics');
const extra = process.argv.slice(2);

const r = spawnSync(bin, extra, {
  cwd: root,
  stdio: 'inherit',
  windowsHide: true,
});

if (r.error) {
  console.error('Tokalytics: não foi possível executar', bin, '-', r.error.message);
  process.exit(1);
}

process.exit(typeof r.status === 'number' ? r.status : 1);
