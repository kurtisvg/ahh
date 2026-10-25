import { appURL } from './router.js';

export function link(path, label) {
  const a = document.createElement('a');
  a.href = appURL(path);
  a.dataset.nav = '';
  a.textContent = label;
  return a;
}
