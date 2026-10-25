import { mountTerminal } from './terminal.js';
import { startRouter, appURL } from './router.js';

let activeView = () => {};
export const dispose = () => activeView();

startRouter(() => {
  activeView();
  document.querySelector('#terminal').replaceChildren();
  document.querySelector('#status').textContent = 'Loading…';
  document.querySelector('#start').disabled = false;
  activeView = mountTerminal({
    container: document.querySelector('#terminal'),
    status: document.querySelector('#status'),
    start: document.querySelector('#start'),
    notice: document.querySelector('#notice'),
  });
});

const link = document.querySelector('#terminal-link');
link.href = appURL('terminal');
