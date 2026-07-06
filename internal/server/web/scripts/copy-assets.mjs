import { access, copyFile, mkdir } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const webDir = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const serverDir = resolve(webDir, '..');
const assetsDir = join(serverDir, 'assets');

const assets = [
  {
    candidates: [
      join(webDir, 'src', 'index.html')
    ],
    target: 'index.html'
  },
  {
    candidates: [
      join(webDir, 'node_modules', '@xterm', 'xterm', 'css', 'xterm.css')
    ],
    target: 'xterm.css'
  },
  {
    candidates: [
      join(webDir, 'node_modules', '@xterm', 'xterm', 'lib', 'xterm.js')
    ],
    target: 'xterm.js'
  },
  {
    candidates: [
      join(webDir, 'node_modules', '@xterm', 'addon-fit', 'lib', 'addon-fit.js')
    ],
    target: 'addon-fit.js'
  },
  {
    candidates: [
      join(webDir, 'src', 'app.css')
    ],
    target: 'app.css'
  },
  {
    candidates: [
      join(webDir, 'src', 'app.js')
    ],
    target: 'app.js'
  }
];

await mkdir(assetsDir, { recursive: true });

for (const asset of assets) {
  await copyFirstExisting(asset.candidates, join(assetsDir, asset.target));
}

async function copyFirstExisting(candidates, target) {
  for (const candidate of candidates) {
    try {
      await access(candidate);
      await copyFile(candidate, target);
      return;
    } catch (error) {
      if (error.code !== 'ENOENT') {
        throw error;
      }
    }
  }

  throw new Error(`missing source asset for ${target}`);
}
