const statusEl = document.getElementById('status');
const statusText = document.getElementById('status-text');
const conversationEl = document.querySelector('.conversation-item');
const terminalEl = document.getElementById('terminal');

const terminal = new Terminal({
  cursorBlink: true,
  convertEol: true,
  customGlyphs: true,
  fontFamily: '"DejaVu Sans Mono", "Noto Sans Mono", "Liberation Mono", "Courier New", monospace',
  fontSize: 14,
  fontWeight: 400,
  fontWeightBold: 700,
  letterSpacing: 0,
  lineHeight: 1.15,
  scrollback: 5000,
  theme: {
    background: '#0a0a0a',
    foreground: '#f2f2f2',
    cursor: '#f2f2f2',
    selectionBackground: '#4f5f66'
  }
});
const fitAddon = new FitAddon.FitAddon();
terminal.loadAddon(fitAddon);
terminal.open(terminalEl);

function setStatus(state) {
  statusEl.dataset.state = state;
  conversationEl.dataset.state = state;
  statusText.textContent = state;
}

function terminalSocketURL() {
  const basePath = window.location.pathname.endsWith('/')
    ? window.location.pathname
    : window.location.pathname + '/';
  const url = new URL('pty', window.location.origin + basePath);
  url.protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return url.toString();
}

const socket = new WebSocket(terminalSocketURL());
socket.binaryType = 'arraybuffer';
const decoder = new TextDecoder();

function sendResize() {
  if (socket.readyState !== WebSocket.OPEN) {
    return;
  }
  socket.send(JSON.stringify({
    type: 'resize',
    rows: terminal.rows,
    cols: terminal.cols
  }));
}

function fitTerminal() {
  fitAddon.fit();
  sendResize();
}

socket.addEventListener('open', () => {
  setStatus('connected');
  fitTerminal();
  terminal.focus();
});

socket.addEventListener('message', (event) => {
  if (event.data instanceof ArrayBuffer) {
    terminal.write(decoder.decode(event.data, { stream: true }));
    return;
  }
  terminal.write(event.data);
});

socket.addEventListener('close', () => {
  setStatus('closed');
});

socket.addEventListener('error', () => {
  setStatus('error');
});

terminal.onData((data) => {
  if (socket.readyState === WebSocket.OPEN) {
    socket.send(JSON.stringify({
      type: 'input',
      data
    }));
  }
});

let resizeTimer;
window.addEventListener('resize', () => {
  window.clearTimeout(resizeTimer);
  resizeTimer = window.setTimeout(fitTerminal, 50);
});

fitTerminal();
