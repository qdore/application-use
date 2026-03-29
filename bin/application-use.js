#!/usr/bin/env node

/**
 * Cross-platform CLI wrapper for application-use
 */

import { spawn } from 'child_process';
import { existsSync, accessSync, chmodSync, constants } from 'fs';
import { dirname, join } from 'path';
import { fileURLToPath } from 'url';
import { platform, arch } from 'os';

const __dirname = dirname(fileURLToPath(import.meta.url));

// Map Node.js platform/arch to binary naming convention
function getBinaryName() {
  const os = platform();
  const cpuArch = arch();

  if (os !== 'darwin') {
    return null;
  }

  let archKey;
  switch (cpuArch) {
    case 'x64':
    case 'x86_64':
      archKey = 'x64';
      break;
    case 'arm64':
    case 'aarch64':
      archKey = 'arm64';
      break;
    default:
      return null;
  }

  return `application-use-darwin-${archKey}`;
}

function main() {
  const binaryName = getBinaryName();

  if (!binaryName) {
    console.error(`Error: Unsupported platform: ${platform()}-${arch()}. application-use only supports macOS.`);
    process.exit(1);
  }

  const binaryPath = join(__dirname, binaryName);

  if (!existsSync(binaryPath)) {
    console.error(`Error: No binary found for ${platform()}-${arch()}`);
    console.error(`Expected: ${binaryPath}`);
    console.error('');
    console.error('Please ensure the package was installed correctly or run "make package-npm" to build locally.');
    process.exit(1);
  }

  // Ensure binary is executable
  try {
    accessSync(binaryPath, constants.X_OK);
  } catch {
    try {
      chmodSync(binaryPath, 0o755);
    } catch (chmodErr) {
      console.error(`Error: Cannot make binary executable: ${chmodErr.message}`);
      process.exit(1);
    }
  }

  // Spawn the native binary with inherited stdio
  const child = spawn(binaryPath, process.argv.slice(2), {
    stdio: 'inherit',
    windowsHide: false,
  });

  child.on('error', (err) => {
    console.error(`Error executing binary: ${err.message}`);
    process.exit(1);
  });

  child.on('close', (code) => {
    process.exit(code ?? 0);
  });
}

main();
