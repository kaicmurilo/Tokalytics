#!/usr/bin/env node
'use strict';

const https = require('https');
const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

const REPO = 'kaicmurilo/tokalytics';
const BIN_DIR = path.join(__dirname, '..', 'bin');
const BIN_PATH = path.join(BIN_DIR, process.platform === 'win32' ? 'tokalytics.exe' : 'tokalytics');

function getPlatformBinary() {
  const platform = process.platform;
  const arch = process.arch;

  const platformMap = {
    'darwin-arm64': 'tokalytics-darwin-arm64',
    'darwin-x64':   'tokalytics-darwin-amd64',
    'linux-arm64':  'tokalytics-linux-arm64',
    'linux-x64':    'tokalytics-linux-amd64',
    'win32-x64':    'tokalytics-windows-amd64.exe',
  };

  const key = `${platform}-${arch}`;
  const name = platformMap[key];
  if (!name) {
    throw new Error(`Plataforma não suportada: ${key}`);
  }
  return name;
}

function fetchJson(url) {
  return new Promise((resolve, reject) => {
    const options = { headers: { 'User-Agent': 'tokalytics-installer' } };
    https.get(url, options, (res) => {
      if (res.statusCode === 302 || res.statusCode === 301) {
        return fetchJson(res.headers.location).then(resolve).catch(reject);
      }
      let data = '';
      res.on('data', (chunk) => (data += chunk));
      res.on('end', () => {
        try { resolve(JSON.parse(data)); }
        catch (e) { reject(new Error('Resposta inválida da API do GitHub')); }
      });
    }).on('error', reject);
  });
}

function downloadFile(url, dest) {
  return new Promise((resolve, reject) => {
    const options = { headers: { 'User-Agent': 'tokalytics-installer' } };
    const file = fs.createWriteStream(dest);
    const follow = (u) => {
      https.get(u, options, (res) => {
        if (res.statusCode === 302 || res.statusCode === 301) {
          return follow(res.headers.location);
        }
        if (res.statusCode !== 200) {
          return reject(new Error(`Download falhou: HTTP ${res.statusCode}`));
        }
        res.pipe(file);
        file.on('finish', () => file.close(resolve));
      }).on('error', (err) => {
        fs.unlink(dest, () => {});
        reject(err);
      });
    };
    follow(url);
  });
}

async function install() {
  console.log('Tokalytics: buscando última versão...');

  const release = await fetchJson(`https://api.github.com/repos/${REPO}/releases/latest`);
  const version = release.tag_name;
  const binaryName = getPlatformBinary();

  const asset = release.assets.find((a) => a.name === binaryName);
  if (!asset) {
    throw new Error(`Binário "${binaryName}" não encontrado na release ${version}`);
  }

  console.log(`Tokalytics: baixando ${binaryName} (${version})...`);
  if (!fs.existsSync(BIN_DIR)) fs.mkdirSync(BIN_DIR, { recursive: true });

  await downloadFile(asset.browser_download_url, BIN_PATH);
  fs.chmodSync(BIN_PATH, 0o755);

  console.log(`Tokalytics ${version} instalado com sucesso!`);
  console.log('Execute: tokalytics');
}

install().catch((err) => {
  console.error('Erro na instalação do Tokalytics:', err.message);
  process.exit(1);
});
