import { mountTerminal } from './terminal.js';

export const dispose = mountTerminal({
  container: document.querySelector('#terminal'),
  status: document.querySelector('#status'),
  start: document.querySelector('#start'),
  notice: document.querySelector('#notice'),
});
