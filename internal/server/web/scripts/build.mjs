import { createHash } from 'node:crypto';
import { copyFile, cp, mkdir, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

process.chdir(fileURLToPath(new URL('..', import.meta.url)));
await rm('dist', { recursive: true, force: true });
await mkdir('dist/vendor', { recursive: true });
await cp('src', 'dist', { recursive: true });
for (const [source, target] of [
  ['@xterm/xterm/lib/xterm.js', 'xterm.js'],
  ['@xterm/xterm/css/xterm.css', 'xterm.css'],
  ['@xterm/xterm/LICENSE', 'xterm-LICENSE'],
  ['@xterm/addon-fit/lib/addon-fit.js', 'addon-fit.js'],
  ['@xterm/addon-fit/LICENSE', 'addon-fit-LICENSE'],
  ['@xterm/addon-image/lib/addon-image.js', 'addon-image.js'],
  ['@xterm/addon-image/LICENSE', 'addon-image-LICENSE'],
  ['@xterm/addon-clipboard/lib/addon-clipboard.js', 'addon-clipboard.js'],
  ['@xterm/addon-clipboard/LICENSE', 'addon-clipboard-LICENSE'],
  ['js-base64/LICENSE.md', 'js-base64-LICENSE'],
]) {
  await copyFile(`node_modules/${source}`, `dist/vendor/${target}`);
}

// Version the whole module graph so a cached router cannot survive a deployment.
const sources = (await readdir('src')).sort();
const digest = createHash('sha256').update(await readFile('package-lock.json'));
for (const name of sources) digest.update(name).update(await readFile('src/' + name));
const revision = digest.digest('hex').slice(0, 16);
for (const name of sources) {
  let source = await readFile('src/' + name, 'utf8');
  if (name.endsWith('.js')) {
    source = source.replace(/(from ['"])(\.\/[^'"]+\.js)(['"])/g,
      (_, before, path, after) => before + path + '?v=' + revision + after);
  } else if (name === 'index.html') {
    source = source.replace(/((?:src|href)="[^"?]+\.(?:js|css))"/g,
      (_, path) => path + '?v=' + revision + '"');
  }
  await writeFile('dist/' + name, source);
}
