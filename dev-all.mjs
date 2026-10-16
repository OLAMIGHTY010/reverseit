#!/usr/bin/env node
/**
 * SMARTREVERSAL Multi-Service Development Orchestrator
 * 
 * Concurrently manages:
 *   1. Core Go Backend API       (:8080)
 *   2. Python FastAPI AI Service (:8000)
 *   3. React Vite Web Console    (:5173)
 *   4. (Optional) Outbox Worker  (:8081)
 * 
 * Usage:
 *   node dev-all.mjs               # Starts API, AI Service, and Web Console
 *   node dev-all.mjs --migrate     # Runs database migrations before starting
 *   node dev-all.mjs --with-worker # Starts backend with outbox background worker
 *   node dev-all.mjs --api-only    # Starts only the Go API backend
 *   node dev-all.mjs --ai-only     # Starts only the Python AI service
 *   node dev-all.mjs --web-only    # Starts only the React frontend
 */

import { spawn, execSync } from 'node:child_process';
import { existsSync, readFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import readline from 'node:readline';
import http from 'node:http';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const ROOT_DIR = __dirname;

// ANSI Colors
const c = {
  reset: '\x1b[0m',
  bold: '\x1b[1m',
  dim: '\x1b[2m',
  cyan: '\x1b[36m',
  magenta: '\x1b[35m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  blue: '\x1b[34m',
  red: '\x1b[31m',
  gray: '\x1b[90m',
};

// Prefixes
const prefixes = {
  SYSTEM: `${c.bold}${c.blue}[SYSTEM]${c.reset} `,
  API:    `${c.bold}${c.cyan}[API]   ${c.reset} `,
  AI:     `${c.bold}${c.magenta}[AI]    ${c.reset} `,
  WEB:    `${c.bold}${c.green}[WEB]   ${c.reset} `,
  WORKER: `${c.bold}${c.yellow}[WORKER]${c.reset} `,
};

// Standalone .env parser
function loadEnv() {
  const envPath = resolve(ROOT_DIR, '.env');
  if (!existsSync(envPath)) return;
  const content = readFileSync(envPath, 'utf8');
  for (const rawLine of content.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line || line.startsWith('#')) continue;
    const eqIdx = line.indexOf('=');
    if (eqIdx === -1) continue;
    const key = line.slice(0, eqIdx).trim();
    let val = line.slice(eqIdx + 1).trim();
    if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) {
      val = val.slice(1, -1);
    }
    if (!process.env[key]) {
      process.env[key] = val;
    }
  }
}

loadEnv();

const isWindows = process.platform === 'win32';
const processes = [];
let isShuttingDown = false;

function sysLog(msg) {
  console.log(`${prefixes.SYSTEM}${msg}`);
}

function sysSuccess(msg) {
  console.log(`${prefixes.SYSTEM}${c.green}✔ ${msg}${c.reset}`);
}

function sysWarn(msg) {
  console.log(`${prefixes.SYSTEM}${c.yellow}⚠ ${msg}${c.reset}`);
}

function sysError(msg) {
  console.error(`${prefixes.SYSTEM}${c.red}✖ ${msg}${c.reset}`);
}

// Kill child process and its entire sub-tree
function killProcessTree(proc) {
  if (!proc || !proc.pid) return;
  try {
    if (isWindows) {
      execSync(`taskkill /pid ${proc.pid} /T /F`, { stdio: 'ignore' });
    } else {
      process.kill(-proc.pid, 'SIGTERM');
    }
  } catch {
    // Process already exited
  }
}

// Graceful cleanup handler
function shutdown(exitCode = 0) {
  if (isShuttingDown) return;
  isShuttingDown = true;
  console.log(`\n${prefixes.SYSTEM}${c.yellow}Shutting down all SMARTREVERSAL services...${c.reset}`);

  for (const { name, proc } of processes) {
    sysLog(`Terminating ${name} (PID: ${proc.pid})...`);
    killProcessTree(proc);
  }

  sysSuccess('All services terminated cleanly.');
  process.exit(exitCode);
}

process.on('SIGINT', () => shutdown(0));
process.on('SIGTERM', () => shutdown(0));
process.on('SIGHUP', () => shutdown(0));

// Spawn a managed service with prefixed pipe streaming
function startService({ name, command, args, cwd, prefixColor, env = {} }) {
  const prefix = prefixes[name] || `[${name}] `;
  sysLog(`Starting ${c.bold}${name}${c.reset} in ${cwd}...`);

  const proc = spawn(command, args, {
    cwd,
    env: { ...process.env, ...env },
    shell: false,
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  processes.push({ name, proc });

  const rlOut = readline.createInterface({ input: proc.stdout });
  rlOut.on('line', (line) => {
    if (line.trim()) console.log(`${prefix}${line}`);
  });

  const rlErr = readline.createInterface({ input: proc.stderr });
  rlErr.on('line', (line) => {
    if (line.trim()) console.error(`${prefix}${c.dim}${line}${c.reset}`);
  });

  proc.on('close', (code) => {
    if (!isShuttingDown) {
      if (code === 0) {
        sysLog(`${name} exited normally.`);
      } else {
        sysWarn(`${name} process exited with code ${code}.`);
      }
    }
  });

  proc.on('error', (err) => {
    sysError(`Failed to start ${name}: ${err.message}`);
  });

  return proc;
}

// Probe HTTP health endpoint
function pollHealth(url, timeoutMs = 2000) {
  return new Promise((resolve) => {
    const req = http.get(url, { timeout: timeoutMs }, (res) => {
      resolve(res.statusCode >= 200 && res.statusCode < 400);
    });
    req.on('error', () => resolve(false));
    req.on('timeout', () => {
      req.destroy();
      resolve(false);
    });
  });
}

async function monitorHealth({ name, url, intervalMs = 1500, maxAttempts = 30 }) {
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    if (isShuttingDown) return;
    const isHealthy = await pollHealth(url);
    if (isHealthy) {
      sysSuccess(`${c.bold}${name}${c.reset} is live & healthy at ${c.cyan}${url}${c.reset}`);
      return true;
    }
    await new Promise((r) => setTimeout(r, intervalMs));
  }
  sysWarn(`${name} healthcheck at ${url} did not respond within expected time.`);
  return false;
}

async function main() {
  const flags = new Set(process.argv.slice(2));

  console.log(`\n${c.bold}${c.cyan}==================================================================${c.reset}`);
  console.log(`${c.bold}${c.cyan}  SMARTREVERSAL Enterprise Multi-Tier Development Environment    ${c.reset}`);
  console.log(`${c.bold}${c.cyan}==================================================================${c.reset}\n`);

  const apiPort = process.env.API_PORT || '8080';
  const aiPort = process.env.AI_SERVICE_PORT || '8000';
  const webPort = '5173';

  sysLog(`Configured Service Endpoints:`);
  console.log(`  ${c.cyan}• Core API Backend:${c.reset}        http://localhost:${apiPort}`);
  console.log(`  ${c.magenta}• AI Intelligence Service:${c.reset} http://localhost:${aiPort}`);
  console.log(`  ${c.green}• Web Operations Console:${c.reset}  http://localhost:${webPort}\n`);

  // Run migrations first if requested
  if (flags.has('--migrate')) {
    sysLog('Running database migrations first (--migrate flag detected)...');
    try {
      execSync('node migrate.mjs up', { cwd: ROOT_DIR, stdio: 'inherit' });
      sysSuccess('Migrations applied successfully.\n');
    } catch (err) {
      sysError('Migration step failed. Aborting dev server launch.');
      process.exit(1);
    }
  }

  const runAll = !flags.has('--api-only') && !flags.has('--ai-only') && !flags.has('--web-only');

  // 1. Core Go Backend API
  if (runAll || flags.has('--api-only')) {
    startService({
      name: 'API',
      command: 'go',
      args: ['run', './cmd/api/main.go'],
      cwd: resolve(ROOT_DIR, 'apps/api'),
    });
    monitorHealth({ name: 'Go API', url: `http://localhost:${apiPort}/v1/health` });
  }

  // 2. Python FastAPI AI Service
  if (runAll || flags.has('--ai-only')) {
    startService({
      name: 'AI',
      command: 'python',
      args: ['-m', 'uvicorn', 'app.main:app', '--host', '0.0.0.0', '--port', aiPort, '--reload'],
      cwd: resolve(ROOT_DIR, 'apps/ai-service'),
    });
    monitorHealth({ name: 'AI Service', url: `http://localhost:${aiPort}/v1/ai/health` });
  }

  // 3. Optional Outbox Worker
  if (flags.has('--with-worker')) {
    startService({
      name: 'WORKER',
      command: 'go',
      args: ['run', './cmd/worker/main.go'],
      cwd: resolve(ROOT_DIR, 'apps/api'),
    });
  }

  // 4. React Vite Web Operations Console
  if (runAll || flags.has('--web-only')) {
    startService({
      name: 'WEB',
      command: isWindows ? 'npx.cmd' : 'npx',
      args: ['vite', '--port', webPort, '--host'],
      cwd: resolve(ROOT_DIR, 'apps/web'),
    });
    monitorHealth({ name: 'Web Console', url: `http://localhost:${webPort}` });
  }

  console.log(`\n${prefixes.SYSTEM}${c.bold}Press Ctrl+C to terminate all services cleanly.${c.reset}\n`);
}

main().catch((err) => {
  sysError(`Fatal startup error: ${err.message}`);
  shutdown(1);
});
