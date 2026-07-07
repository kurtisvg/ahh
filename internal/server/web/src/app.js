const appShell = document.querySelector('.app-shell');
const statusEl = document.getElementById('status');
const statusText = document.getElementById('status-text');
const statusDetail = document.getElementById('status-detail');
const connectionBanner = document.getElementById('connection-banner');
const connectionBannerText = document.getElementById('connection-banner-text');
const conversationEl = document.querySelector('.conversation-item');
const terminalEl = document.getElementById('terminal');
const terminalState = document.getElementById('terminal-state');
const terminalStateText = document.getElementById('terminal-state-text');

const reconnectBaseDelay = 500;
const reconnectMaxDelay = 5000;
const readinessPollInterval = 3000;
const stateLabels = {
  connecting: 'connecting',
  connected: 'connected',
  disconnected: 'disconnected',
  reconnecting: 'reconnecting',
  error: 'error',
  'harness-exited': 'harness exited'
};
const connectionDetails = {
  connecting: 'opening websocket',
  connected: 'terminal connected',
  disconnected: 'socket disconnected',
  reconnecting: 'retrying connection',
  error: 'connection error',
  'harness-exited': 'terminal stopped'
};
const readinessLabels = {
  checking: 'checking',
  ready: 'ready',
  unavailable: 'unavailable',
  exited: 'harness exited'
};
const terminalStateMessages = {
  connecting: 'Connecting to terminal',
  disconnected: 'Terminal disconnected',
  reconnecting: 'Reconnecting to terminal',
  error: 'Terminal connection error',
  'harness-exited': 'Harness exited'
};

let socket;
let connectionState = 'connecting';
let readinessState = 'checking';
let reconnectAttempt = 0;
let reconnectTimer;
let readinessTimer;
let harnessExited = false;
let hasTerminalOutput = false;

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
  connectionState = state;
  statusEl.dataset.state = state;
  conversationEl.dataset.state = state;
  statusText.textContent = stateLabels[state] || state;
  updateStatusDetail();
  updateTerminalState();
}

function setReadinessState(state) {
  readinessState = state;
  appShell.dataset.readyState = state;
  updateStatusDetail();
}

function updateStatusDetail() {
  const detail = statusDetailText();
  statusDetail.textContent = detail;
  statusDetail.hidden = detail === '';
}

function statusDetailText() {
  if (connectionState === 'connected' && readinessState === 'ready') {
    return '';
  }

  return readinessLabels[readinessState] || connectionDetails[connectionState] || '';
}

function updateTerminalState() {
  if (hasTerminalOutput) {
    terminalState.hidden = true;
    return;
  }

  const message = terminalStateMessages[connectionState];
  terminalState.hidden = !message;
  terminalState.dataset.state = connectionState;
  terminalStateText.textContent = message || '';
}

function showBanner(state, message) {
  connectionBanner.dataset.state = state;
  connectionBannerText.textContent = message;
  connectionBanner.hidden = false;
}

function hideBanner() {
  connectionBanner.hidden = true;
}

function appURL(path) {
  const basePath = window.location.pathname.endsWith('/')
    ? window.location.pathname
    : window.location.pathname + '/';
  return new URL(path, window.location.origin + basePath);
}

function terminalSocketURL() {
  const url = appURL('pty');
  url.protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return url.toString();
}

const decoder = new TextDecoder();

function sendResize() {
  if (!socket || socket.readyState !== WebSocket.OPEN) {
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

function connectSocket() {
  if (harnessExited) {
    return;
  }

  setStatus(reconnectAttempt === 0 ? 'connecting' : 'reconnecting');
  socket = new WebSocket(terminalSocketURL());
  socket.binaryType = 'arraybuffer';
  const activeSocket = socket;

  activeSocket.addEventListener('open', () => {
    if (activeSocket !== socket || harnessExited) {
      return;
    }
    reconnectAttempt = 0;
    setStatus('connected');
    hideBanner();
    fitTerminal();
    terminal.focus();
  });

  activeSocket.addEventListener('message', (event) => {
    if (activeSocket !== socket || harnessExited) {
      return;
    }
    if (event.data instanceof ArrayBuffer) {
      hasTerminalOutput = true;
      updateTerminalState();
      terminal.write(decoder.decode(event.data, { stream: true }));
      return;
    }
    hasTerminalOutput = true;
    updateTerminalState();
    terminal.write(event.data);
  });

  activeSocket.addEventListener('close', () => {
    if (activeSocket !== socket || harnessExited) {
      return;
    }
    handleSocketClose();
  });

  activeSocket.addEventListener('error', () => {
    if (activeSocket === socket && !harnessExited) {
      setStatus('disconnected');
    }
  });
}

async function handleSocketClose() {
  setStatus('disconnected');
  showBanner('disconnected', 'Terminal connection dropped. Checking harness state.');

  const readyState = await readReadyState();
  setReadinessState(readyState);
  if (readyState === 'exited') {
    markHarnessExited();
    return;
  }

  scheduleReconnect();
}

async function readReadyState() {
  try {
    const response = await fetch(appURL('ready'), { cache: 'no-store' });
    if (response.status === 503) {
      return 'exited';
    }
    if (response.ok) {
      return 'ready';
    }
  } catch {
    return 'unavailable';
  }

  return 'unavailable';
}

function scheduleReconnect() {
  reconnectAttempt += 1;
  const delay = Math.min(reconnectBaseDelay * 2 ** (reconnectAttempt - 1), reconnectMaxDelay);
  setStatus('reconnecting');
  showBanner('reconnecting', `Terminal disconnected. Reconnecting in ${formatDelay(delay)}.`);
  window.clearTimeout(reconnectTimer);
  reconnectTimer = window.setTimeout(connectSocket, delay);
}

function formatDelay(delay) {
  return `${Math.ceil(delay / 1000)}s`;
}

function markHarnessExited() {
  if (harnessExited) {
    return;
  }

  harnessExited = true;
  hasTerminalOutput = true;
  window.clearTimeout(reconnectTimer);
  window.clearInterval(readinessTimer);
  if (socket && socket.readyState !== WebSocket.CLOSED) {
    socket.close();
  }
  setReadinessState('exited');
  setStatus('harness-exited');
  showBanner('harness-exited', 'Harness exited. Terminal input is no longer connected.');
  updateTerminalState();
  terminal.writeln('');
  terminal.writeln('Ahh: harness exited. Terminal input is no longer connected.');
}

async function refreshReadiness() {
  if (harnessExited) {
    return;
  }

  const readyState = await readReadyState();
  setReadinessState(readyState);
  if (readyState === 'exited') {
    markHarnessExited();
  }
}

function startReadinessPolling() {
  void refreshReadiness();
  readinessTimer = window.setInterval(refreshReadiness, readinessPollInterval);
}

terminal.onData((data) => {
  if (socket && socket.readyState === WebSocket.OPEN && !harnessExited) {
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
startReadinessPolling();
connectSocket();
