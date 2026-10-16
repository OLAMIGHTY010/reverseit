#!/usr/bin/env node
/**
 * SMARTREVERSAL Migration & Database Management Runner
 * 
 * Usage:
 *   node migrate.mjs            # Applies all pending migrations (default: up)
 *   node migrate.mjs up         # Applies all pending migrations
 *   node migrate.mjs down [N]   # Reverts N migrations (default 1)
 *   node migrate.mjs status     # Displays current migration status
 *   node migrate.mjs seed       # Seeds the demo institution, operators, and test policies
 *   node migrate.mjs reset      # Resets demo scenario data
 *   node migrate.mjs play       # Replays demo transaction scenarios
 */

import { spawn } from 'node:child_process';
import { existsSync, readFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import net from 'node:net';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const ROOT_DIR = __dirname;
const API_DIR = resolve(ROOT_DIR, 'apps/api');

// ANSI color helpers
const c = {
  reset: '\x1b[0m',
  bold: '\x1b[1m',
  dim: '\x1b[2m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  blue: '\x1b[34m',
  magenta: '\x1b[35m',
  cyan: '\x1b[36m',
  red: '\x1b[31m',
};

function log(msg) {
  console.log(`${c.blue}[MIGRATE]${c.reset} ${msg}`);
}

function success(msg) {
  console.log(`${c.green}[MIGRATE] ✔ ${msg}${c.reset}`);
}

function warn(msg) {
  console.log(`${c.yellow}[MIGRATE] ⚠ ${msg}${c.reset}`);
}

function error(msg) {
  console.error(`${c.red}[MIGRATE] ✖ ${msg}${c.reset}`);
}

// Simple standalone .env parser
function loadEnv() {
  const envPath = resolve(ROOT_DIR, '.env');
  if (!existsSync(envPath)) {
    warn('No .env file found at project root. Using system environment variables.');
    return;
  }
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

// TCP port readiness check
function checkTcp(host, port, timeoutMs = 2000) {
  return new Promise((res) => {
    const socket = new net.Socket();
    let isConnected = false;
    socket.setTimeout(timeoutMs);
    socket.once('connect', () => {
      isConnected = true;
      socket.destroy();
      res(true);
    });
    socket.once('error', () => {
      socket.destroy();
      res(false);
    });
    socket.once('timeout', () => {
      socket.destroy();
      res(false);
    });
    socket.connect(port, host);
  });
}

async function runCommand(cmd, args, cwd, env) {
  return new Promise((res, rej) => {
    const proc = spawn(cmd, args, {
      cwd,
      env: { ...process.env, ...env },
      stdio: 'inherit',
      shell: false,
    });
    proc.on('close', (code) => {
      if (code === 0) res();
      else rej(new Error(`Command exited with status code ${code}`));
    });
    proc.on('error', rej);
  });
}

async function main() {
  const args = process.argv.slice(2);
  const command = args[0] || 'up';
  const extraArgs = args.slice(1);

  console.log(`\n${c.bold}${c.cyan}====================================================${c.reset}`);
  console.log(`${c.bold}${c.cyan}  SMARTREVERSAL Database & Migration Engine         ${c.reset}`);
  console.log(`${c.bold}${c.cyan}====================================================${c.reset}\n`);

  loadEnv();

  const dbHost = process.env.DB_HOST || 'localhost';
  const dbPort = parseInt(process.env.DB_PORT || '5432', 10);
  const dbName = process.env.DB_NAME || 'smartreversal_db';
  const dbUser = process.env.DB_USER || 'postgres';

  log(`Target Database: ${c.bold}${dbUser}@${dbHost}:${dbPort}/${dbName}${c.reset}`);
  log(`Command: ${c.bold}${command} ${extraArgs.join(' ')}${c.reset}`);

  // Test database connection
  log(`Verifying PostgreSQL TCP connectivity on ${dbHost}:${dbPort}...`);
  const isAvailable = await checkTcp(dbHost, dbPort);
  if (!isAvailable) {
    warn(`PostgreSQL does not appear to be listening on ${dbHost}:${dbPort}.`);
    warn(`Please ensure PostgreSQL is running (e.g. docker compose up -d postgres) before running migrations.\n`);
  } else {
    success(`PostgreSQL TCP port ${dbPort} is reachable.`);
  }

  try {
    if (['up', 'down', 'status'].includes(command)) {
      log(`Executing migration command: ${c.bold}go run ./cmd/migrate ${command} ${extraArgs.join(' ')}${c.reset}...`);
      await runCommand('go', ['run', './cmd/migrate', command, ...extraArgs], API_DIR, {
        ENVIRONMENT: process.env.ENVIRONMENT || 'development',
      });
      success(`Migration '${command}' completed successfully!`);
    } else if (['seed', 'play', 'reset', 'code'].includes(command)) {
      log(`Executing demo command: ${c.bold}go run ./cmd/demo ${command} ${extraArgs.join(' ')}${c.reset}...`);
      await runCommand('go', ['run', './cmd/demo', command, ...extraArgs], API_DIR, {
        DEMO_PASSWORD: process.env.DEMO_PASSWORD || 'DemoSecret123!',
      });
      success(`Demo scenario '${command}' executed successfully!`);
    } else {
      error(`Unknown command: "${command}". Available commands: up, down [steps], status, seed, reset, play.`);
      process.exit(1);
    }
  } catch (err) {
    error(`Migration failed: ${err.message}`);
    process.exit(1);
  }
}

main();
