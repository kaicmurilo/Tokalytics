#!/usr/bin/env node
'use strict';

const http = require('http');
const https = require('https');
const fs = require('fs');
const path = require('path');
const { spawn } = require('child_process');

/** Mesma faixa que `pkg/instancectl` (dashboard HTTP). */
const PORT_MIN = 3456;
const PORT_MAX = 3555;
const SERVICE_NAME = 'tokalytics';

function isGlobalNpmInstall() {
  const g = process.env.npm_config_global;
  return g === 'true' || g === '1';
}

function httpGetJson(url, timeoutMs) {
  return new Promise((resolve) => {
    const req = http.get(url, { timeout: timeoutMs }, (res) => {
      let data = '';
      res.on('data', (chunk) => {
        data += chunk;
      });
      res.on('end', () => {
        if (res.statusCode !== 200) return resolve(null);
        try {
          resolve(JSON.parse(data));
        } catch {
          resolve(null);
        }
      });
    });
    req.on('error', () => resolve(null));
    req.on('timeout', () => {
      req.destroy();
      resolve(null);
    });
  });
}

/** Retorna a porta do primeiro Tokalytics que responder a /api/health, ou 0. */
async function findTokalyticsPort() {
  for (let p = PORT_MIN; p <= PORT_MAX; p++) {
    const j = await httpGetJson(`http://127.0.0.1:${p}/api/health`, 400);
    if (j && j.service === SERVICE_NAME) return p;
  }
  return 0;
}

function httpPostShutdown(port) {
  return new Promise((resolve) => {
    const req = http.request(
      {
        hostname: '127.0.0.1',
        port,
        path: '/api/shutdown',
        method: 'POST',
        timeout: 4000,
      },
      () => resolve()
    );
    req.on('error', () => resolve());
    req.on('timeout', () => {
      req.destroy();
      resolve();
    });
    req.end();
  });
}

/**
 * Encerra instância em execução (npm update / install -g) antes de sobrescrever bin/tokalytics.
 * Usa HTTP local — não depende do shim `tokalytics` no PATH.
 */
async function stopRunningInstanceBeforeBinaryReplace() {
  const port = await findTokalyticsPort();
  if (!port) return;
  console.log('Tokalytics: encerrando instância em execução antes da atualização...');
  await httpPostShutdown(port);
  const deadline = Date.now() + 25000;
  while (Date.now() < deadline) {
    const still = await findTokalyticsPort();
    if (!still) return;
    await new Promise((r) => setTimeout(r, 100));
  }
  console.warn(
    'Tokalytics: aviso — instância anterior pode ainda estar ativa; se o download falhar, use tokalytics --stop e rode npm de novo.'
  );
}

const pkgPath = path.join(__dirname, '..', 'package.json');
const pkgVersion = (() => {
  try {
    return JSON.parse(fs.readFileSync(pkgPath, 'utf8')).version || '?';
  } catch {
    return '?';
  }
})();

// Repositório público no GitHub (nome real: Tokalytics)
const REPO = 'kaicmurilo/Tokalytics';
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

function fetchJson(url, extraHeaders = {}) {
  return new Promise((resolve, reject) => {
    const headers = {
      'User-Agent': 'tokalytics-installer',
      Accept: 'application/vnd.github+json',
      ...extraHeaders,
    };
    if (process.env.GITHUB_TOKEN) {
      headers.Authorization = `Bearer ${process.env.GITHUB_TOKEN}`;
    }
    const options = { headers };
    https.get(url, options, (res) => {
      if (res.statusCode === 302 || res.statusCode === 301) {
        return fetchJson(res.headers.location).then(resolve).catch(reject);
      }
      let data = '';
      res.on('data', (chunk) => (data += chunk));
      res.on('end', () => {
        if (res.statusCode && res.statusCode >= 400) {
          try {
            const j = JSON.parse(data);
            if (j && j.message) {
              return reject(
                new Error(`GitHub HTTP ${res.statusCode}: ${j.message}`)
              );
            }
          } catch (_) {
            /* fallthrough */
          }
          return reject(new Error(`GitHub HTTP ${res.statusCode}`));
        }
        try {
          resolve(JSON.parse(data));
        } catch {
          reject(new Error('Resposta inválida da API do GitHub'));
        }
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

function assertReleasePayload(release) {
  if (!release || typeof release !== 'object') {
    throw new Error('Resposta inválida da API do GitHub (corpo vazio).');
  }
  if (release.message && !release.tag_name) {
    let hint = '';
    if (/rate limit/i.test(release.message) && !process.env.GITHUB_TOKEN) {
      hint = ' Defina GITHUB_TOKEN no ambiente para aumentar o limite.';
    } else if (/not found/i.test(release.message)) {
      hint =
        ` Confirme que https://github.com/${REPO} existe, é público e tem pelo menos uma release com binários anexados.`;
    }
    throw new Error(`GitHub: ${release.message}${hint}`);
  }
  if (!release.tag_name) {
    throw new Error('Release sem tag_name; verifique se existe release no repositório.');
  }
  if (!Array.isArray(release.assets)) {
    throw new Error(
      'Resposta da API sem lista de assets. Possível rate limit ou repositório/release inexistente.'
    );
  }
}

/** Só em `npm install -g` / `npm update -g` (npm_config_global). Ignora CI e TOKALYTICS_NO_AUTOSTART=1. */
function tryLaunchAfterGlobalInstall() {
  if (process.env.CI === 'true') return false;
  if (process.env.TOKALYTICS_NO_AUTOSTART === '1') return false;
  if (!isGlobalNpmInstall()) return false;
  if (!fs.existsSync(BIN_PATH)) return false;
  try {
    const child = spawn(BIN_PATH, ['-start'], {
      detached: true,
      stdio: 'ignore',
      windowsHide: true,
    });
    child.unref();
    return true;
  } catch {
    return false;
  }
}

async function install() {
  console.log(`Tokalytics installer v${pkgVersion}: buscando última versão...`);

  if (isGlobalNpmInstall() && process.env.CI !== 'true') {
    await stopRunningInstanceBeforeBinaryReplace();
  }

  const release = await fetchJson(`https://api.github.com/repos/${REPO}/releases/latest`);
  assertReleasePayload(release);
  const version = release.tag_name;
  const binaryName = getPlatformBinary();

  const assets = Array.isArray(release.assets) ? release.assets : [];
  const asset = assets.find((a) => a && a.name === binaryName);
  if (!asset) {
    throw new Error(`Binário "${binaryName}" não encontrado na release ${version}`);
  }

  console.log(`Tokalytics: baixando ${binaryName} (${version})...`);
  if (!fs.existsSync(BIN_DIR)) fs.mkdirSync(BIN_DIR, { recursive: true });

  await downloadFile(asset.browser_download_url, BIN_PATH);
  fs.chmodSync(BIN_PATH, 0o755);

  console.log(`Tokalytics ${version} instalado com sucesso!`);
  if (tryLaunchAfterGlobalInstall()) {
    console.log(
      'Tokalytics iniciado em segundo plano (tokalytics -start). Encerre com: tokalytics --stop'
    );
  } else {
    console.log('Execute: tokalytics ou tokalytics -start');
  }
}

install().catch((err) => {
  console.error('Erro na instalação do Tokalytics:', err.message);
  process.exit(1);
});
