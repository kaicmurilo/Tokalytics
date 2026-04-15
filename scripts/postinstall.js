#!/usr/bin/env node
'use strict';

const http = require('http');
const https = require('https');
const fs = require('fs');
const os = require('os');
const path = require('path');
const { spawn, spawnSync } = require('child_process');

/** Mesma faixa que `pkg/instancectl` (dashboard HTTP). */
const PORT_MIN = 3456;
const PORT_MAX = 3555;
const SERVICE_NAME = 'tokalytics';

const BIN_DIR = path.join(__dirname, '..', 'bin');
const BIN_PATH = path.join(BIN_DIR, process.platform === 'win32' ? 'tokalytics.exe' : 'tokalytics');

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
        timeout: 8000,
      },
      (res) => {
        res.resume();
        res.on('end', () => resolve());
      }
    );
    req.on('error', () => resolve());
    req.on('timeout', () => {
      req.destroy();
      resolve();
    });
    req.end();
  });
}

function runstatePath() {
  return path.join(os.homedir(), '.config', 'tokalytics', 'runstate.json');
}

function readRunstatePid() {
  try {
    const raw = fs.readFileSync(runstatePath(), 'utf8');
    const j = JSON.parse(raw);
    const pid = Number(j.pid) || 0;
    return pid > 0 ? pid : 0;
  } catch {
    return 0;
  }
}

function isProcessAlive(pid) {
  if (!pid) return false;
  try {
    process.kill(pid, 0);
    return true;
  } catch (e) {
    return e.code === 'EPERM';
  }
}

/** Mesmo fluxo que `tokalytics --stop` (usa runstate + HTTP). Só existe em atualização (binário antigo). */
function tryStopViaExistingBinary() {
  if (!fs.existsSync(BIN_PATH)) return;
  console.log('Tokalytics: encerrando instância (binário instalado: --stop)...');
  spawnSync(BIN_PATH, ['--stop'], {
    encoding: 'utf8',
    timeout: 20000,
    windowsHide: true,
    stdio: ['ignore', 'pipe', 'pipe'],
  });
}

/** Windows costuma manter o .exe bloqueado se o processo não morrer; último recurso. */
async function tryKillPidForceAsync(pid) {
  if (!pid || !isProcessAlive(pid)) return;
  console.warn(`Tokalytics: forçando encerramento do PID ${pid} para liberar o binário...`);
  if (process.platform === 'win32') {
    spawnSync('taskkill', ['/PID', String(pid), '/F', '/T'], {
      stdio: 'ignore',
      windowsHide: true,
      timeout: 15000,
    });
    return;
  }
  try {
    process.kill(pid, 'SIGTERM');
  } catch (_) {}
  await new Promise((r) => setTimeout(r, 500));
  if (isProcessAlive(pid)) {
    try {
      process.kill(pid, 'SIGKILL');
    } catch (_) {}
  }
}

async function waitUntilNoTokalyticsHealth(maxMs) {
  const deadline = Date.now() + maxMs;
  while (Date.now() < deadline) {
    const p = await findTokalyticsPort();
    if (!p) return true;
    await new Promise((r) => setTimeout(r, 120));
  }
  return false;
}

/**
 * Encerra instância antes de sobrescrever bin/tokalytics (evita falha de download no Windows
 * e garante que o autostart use o binário novo).
 */
async function stopRunningInstanceBeforeBinaryReplace() {
  tryStopViaExistingBinary();
  let gone = await waitUntilNoTokalyticsHealth(28000);
  if (!gone) {
    const port = await findTokalyticsPort();
    if (port) {
      console.log('Tokalytics: encerrando via HTTP (shutdown)...');
      await httpPostShutdown(port);
      gone = await waitUntilNoTokalyticsHealth(12000);
    }
  }
  const pid = readRunstatePid();
  if (!gone && pid && isProcessAlive(pid)) {
    await tryKillPidForceAsync(pid);
    await waitUntilNoTokalyticsHealth(10000);
  }
  if (await findTokalyticsPort()) {
    console.warn(
      'Tokalytics: aviso — ainda há uma instância respondendo. Se a atualização falhar ou o app não reiniciar, feche o Tokalytics e rode: npm install -g tokalytics'
    );
  }
}

async function waitUntilHealthAppears(maxMs) {
  const deadline = Date.now() + maxMs;
  while (Date.now() < deadline) {
    const p = await findTokalyticsPort();
    if (p) return p;
    await new Promise((r) => setTimeout(r, 200));
  }
  return 0;
}

/** Confirma que o autostart (-start) de fato subiu o HTTP. */
async function confirmAutostartWorked() {
  const p = await waitUntilHealthAppears(28000);
  if (p) {
    console.log(`Tokalytics: confirmado em http://localhost:${p}/`);
    return;
  }
  console.warn(
    'Tokalytics: não foi possível confirmar o início automático. Abra um terminal e execute: tokalytics -start'
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
    const child = spawn(BIN_PATH, [], {
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
      'Tokalytics iniciado com menu bar. Encerre com: tokalytics --stop'
    );
    await confirmAutostartWorked();
  } else {
    console.log('Execute: tokalytics ou tokalytics -start');
  }
}

install().catch((err) => {
  console.error('Erro na instalação do Tokalytics:', err.message);
  process.exit(1);
});
