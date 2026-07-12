const appShell = document.querySelector('.app-shell');
const statusEl = document.getElementById('status');
const statusText = document.getElementById('status-text');
const statusDetail = document.getElementById('status-detail');
const connectionBanner = document.getElementById('connection-banner');
const connectionBannerText = document.getElementById('connection-banner-text');
const conversationListEl = document.getElementById('conversation-list');
const conversationEmptyEl = document.getElementById('conversation-empty');
const newConversationButton = document.getElementById('new-conversation-button');
const conversationDialog = document.getElementById('conversation-dialog');
const createConversationForm = document.getElementById('create-conversation-form');
const conversationDialogClose = document.getElementById('conversation-dialog-close');
const conversationCancelButton = document.getElementById('conversation-cancel-button');
const conversationNameInput = document.getElementById('conversation-name-input');
const viewTitle = document.getElementById('view-title');
const viewSubtitle = document.getElementById('view-subtitle');
const terminalEl = document.getElementById('terminal');
const terminalState = document.getElementById('terminal-state');
const terminalStateText = document.getElementById('terminal-state-text');

const reconnectBaseDelay = 500;
const reconnectMaxDelay = 5000;
const conversationPollInterval = 3000;
const stateLabels = {
  idle: 'idle',
  connecting: 'connecting',
  connected: 'connected',
  disconnected: 'disconnected',
  reconnecting: 'reconnecting',
  error: 'error',
  'conversation-exited': 'not running'
};
const connectionDetails = {
  idle: '',
  connecting: 'opening websocket',
  connected: 'terminal connected',
  disconnected: 'socket disconnected',
  reconnecting: 'retrying connection',
  error: 'connection error',
  'conversation-exited': 'terminal stopped'
};
const terminalStateMessages = {
  idle: 'No conversation selected',
  connecting: 'Connecting to terminal',
  disconnected: 'Terminal disconnected',
  reconnecting: 'Reconnecting to terminal',
  error: 'Terminal connection error',
  'conversation-exited': 'Conversation is not running'
};

let conversations = [];
let activeConversationId = '';
let socket;
let connectionState = 'idle';
let reconnectAttempt = 0;
let reconnectTimer;
let conversationTimer;
let conversationExited = false;
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
  statusText.textContent = stateLabels[state] || state;
  updateStatusDetail();
  updateTerminalState();
}

function updateStatusDetail() {
  const detail = statusDetailText();
  statusDetail.textContent = detail;
  statusDetail.hidden = detail === '';
}

function statusDetailText() {
  const active = activeConversation();
  if (!active) {
    return '';
  }
  if (active.status === 'exited') {
    return 'conversation exited';
  }
  if (connectionState === 'connected') {
    return '';
  }

  return connectionDetails[connectionState] || '';
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
  const basePath = appBasePath();
  return new URL(path, window.location.origin + basePath);
}

function appBasePath() {
  const marker = '/conversations/';
  const markerIndex = window.location.pathname.lastIndexOf(marker);
  if (markerIndex >= 0) {
    return window.location.pathname.slice(0, markerIndex + 1);
  }

  return window.location.pathname.endsWith('/')
    ? window.location.pathname
    : window.location.pathname + '/';
}

function conversationIdFromPath() {
  const prefix = `${appBasePath()}conversations/`;
  if (!window.location.pathname.startsWith(prefix)) {
    return '';
  }

  const rest = window.location.pathname.slice(prefix.length);
  const id = rest.split('/')[0] || '';
  try {
    return decodeURIComponent(id);
  } catch {
    return '';
  }
}

function updateConversationURL(conversationId, mode = 'replace') {
  if (!window.history || !window.history.replaceState) {
    return;
  }

  const url = new URL(window.location.href);
  const basePath = appBasePath();
  url.pathname = conversationId
    ? `${basePath}conversations/${encodeURIComponent(conversationId)}`
    : basePath;
  url.search = '';
  url.hash = '';

  const next = url.pathname + url.search + url.hash;
  const current = window.location.pathname + window.location.search + window.location.hash;
  if (next === current) {
    return;
  }

  const method = mode === 'push' && window.history.pushState ? 'pushState' : 'replaceState';
  window.history[method]({}, '', next);
}

function terminalSocketURL(conversationId) {
  const url = appURL(`api/sessions/${encodeURIComponent(conversationId)}/tty`);
  url.protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return url.toString();
}

const decoder = new TextDecoder();

function activeConversation() {
  return conversations.find((conversation) => conversation.id === activeConversationId);
}

function chooseActiveConversationId({ preserveActive = true } = {}) {
  const requested = conversationIdFromPath();
  if (requested && conversations.some((conversation) => conversation.id === requested)) {
    return requested;
  }

  const activeStillExists = conversations.some((conversation) => conversation.id === activeConversationId);
  if (preserveActive && activeStillExists) {
    return activeConversationId;
  }

  return conversations[0]?.id || '';
}

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

function renderConversations() {
  conversationListEl.replaceChildren();
  conversationEmptyEl.hidden = conversations.length !== 0;
  appShell.dataset.hasConversations = conversations.length === 0 ? 'false' : 'true';

  for (const conversation of conversations) {
    const item = document.createElement('div');
    item.className = 'conversation-item';
    item.dataset.state = conversation.status;
    if (conversation.id === activeConversationId) {
      item.classList.add('is-active');
    }

    const selectButton = document.createElement('button');
    selectButton.className = 'conversation-select';
    selectButton.type = 'button';
    selectButton.ariaCurrent = conversation.id === activeConversationId ? 'page' : 'false';
    selectButton.addEventListener('click', () => selectConversation(conversation.id));

    const main = document.createElement('span');
    main.className = 'conversation-main';

    const name = document.createElement('span');
    name.className = 'conversation-name';
    name.textContent = conversation.name;

    const meta = document.createElement('span');
    meta.className = 'conversation-meta';
    meta.textContent = conversation.status;

    const state = document.createElement('span');
    state.className = 'state-pill';
    state.ariaHidden = 'true';

    main.append(name, meta);
    selectButton.append(main);

    const deleteButton = document.createElement('button');
    deleteButton.className = 'conversation-delete';
    deleteButton.type = 'button';
    deleteButton.innerHTML = `
      <svg class="trash-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
        <path d="M3 6h18"></path>
        <path d="M8 6V4h8v2"></path>
        <path d="M19 6l-1 14H6L5 6"></path>
        <path d="M10 11v5"></path>
        <path d="M14 11v5"></path>
      </svg>
    `;
    deleteButton.setAttribute('aria-label', `Delete ${conversation.name}`);
    deleteButton.addEventListener('click', () => {
      void deleteConversation(conversation.id);
    });

    item.append(selectButton, deleteButton, state);
    conversationListEl.append(item);
  }

  updateViewHeading();
}

function openConversationDialog() {
  conversationNameInput.value = '';
  conversationNameInput.setCustomValidity('');
  hideBanner();
  if (typeof conversationDialog.showModal === 'function') {
    conversationDialog.showModal();
    conversationNameInput.focus();
    return;
  }

  conversationDialog.setAttribute('open', '');
  conversationNameInput.focus();
}

function closeConversationDialog() {
  if (typeof conversationDialog.close === 'function') {
    conversationDialog.close();
    return;
  }

  conversationDialog.removeAttribute('open');
}

function updateViewHeading() {
  const active = activeConversation();
  if (!active) {
    viewTitle.textContent = 'Ahh';
    viewSubtitle.textContent = 'No conversation selected';
    viewSubtitle.hidden = false;
    return;
  }

  viewTitle.textContent = active.name;
  viewSubtitle.textContent = '';
  viewSubtitle.hidden = true;
}

async function loadConversations({ preserveActive = true } = {}) {
  const response = await fetch(appURL('api/sessions'), { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(`list conversations failed: ${response.status}`);
  }

  const payload = await response.json();
  conversations = payload.sessions || [];
  const nextActiveConversationId = chooseActiveConversationId({ preserveActive });
  if (nextActiveConversationId !== activeConversationId) {
    activeConversationId = nextActiveConversationId;
    resetTerminalForActiveConversation();
  }

  renderConversations();
  syncActiveConversation();
  updateConversationURL(activeConversationId);
}

function resetTerminalForActiveConversation() {
  closeSocket();
  terminal.reset();
  hasTerminalOutput = false;
  conversationExited = false;
  reconnectAttempt = 0;
  hideBanner();
}

function syncActiveConversation() {
  const active = activeConversation();
  if (!active) {
    closeSocket();
    setStatus('idle');
    updateViewHeading();
    updateTerminalState();
    return;
  }

  updateViewHeading();
  if (active.status === 'exited') {
    markConversationExited();
    return;
  }
  if (!socket || socket.readyState === WebSocket.CLOSED) {
    connectSocket();
  }
}

function selectConversation(conversationId) {
  if (conversationId === activeConversationId) {
    updateConversationURL(conversationId, 'push');
    return;
  }

  activeConversationId = conversationId;
  resetTerminalForActiveConversation();
  renderConversations();
  syncActiveConversation();
  updateConversationURL(activeConversationId, 'push');
}

async function createConversation(name) {
  const response = await fetch(appURL('api/sessions'), {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({ name })
  });
  if (!response.ok) {
    const payload = await readErrorPayload(response);
    throw new Error(payload || `create conversation failed: ${response.status}`);
  }

  const conversation = await response.json();
  conversations = [conversation, ...conversations.filter((item) => item.id !== conversation.id)];
  activeConversationId = conversation.id;
  resetTerminalForActiveConversation();
  renderConversations();
  syncActiveConversation();
  updateConversationURL(activeConversationId, 'push');
}

async function deleteConversation(conversationId) {
  const response = await fetch(appURL(`api/sessions/${encodeURIComponent(conversationId)}`), {
    method: 'DELETE'
  });
  if (!response.ok && response.status !== 404) {
    const payload = await readErrorPayload(response);
    showBanner('error', payload || 'Conversation could not be deleted.');
    return;
  }

  const wasActive = conversationId === activeConversationId;
  conversations = conversations.filter((conversation) => conversation.id !== conversationId);
  if (wasActive) {
    activeConversationId = conversations[0]?.id || '';
    resetTerminalForActiveConversation();
  }
  renderConversations();
  syncActiveConversation();
  if (wasActive) {
    updateConversationURL(activeConversationId);
  }
}

async function readErrorPayload(response) {
  try {
    const payload = await response.json();
    return payload.error || '';
  } catch {
    return '';
  }
}

function connectSocket() {
  const active = activeConversation();
  if (!active || conversationExited) {
    return;
  }

  setStatus(reconnectAttempt === 0 ? 'connecting' : 'reconnecting');
  socket = new WebSocket(terminalSocketURL(active.id));
  socket.binaryType = 'arraybuffer';
  const activeSocket = socket;

  activeSocket.addEventListener('open', () => {
    if (activeSocket !== socket || conversationExited) {
      return;
    }
    reconnectAttempt = 0;
    setStatus('connected');
    hideBanner();
    fitTerminal();
    terminal.focus();
  });

  activeSocket.addEventListener('message', (event) => {
    if (activeSocket !== socket || conversationExited) {
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
    if (activeSocket !== socket || conversationExited) {
      return;
    }
    void handleSocketClose();
  });

  activeSocket.addEventListener('error', () => {
    if (activeSocket === socket && !conversationExited) {
      setStatus('disconnected');
    }
  });
}

async function handleSocketClose() {
  setStatus('disconnected');
  showBanner('disconnected', 'Terminal connection dropped. Checking conversation state.');

  try {
    await loadConversations();
  } catch {
    scheduleReconnect();
    return;
  }

  const active = activeConversation();
  if (!active || active.status === 'exited') {
    markConversationExited();
    return;
  }

  scheduleReconnect();
}

function scheduleReconnect() {
  if (!activeConversation()) {
    return;
  }

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

function markConversationExited() {
  if (conversationExited && connectionState === 'conversation-exited') {
    return;
  }

  conversationExited = true;
  closeSocket();
  setStatus('conversation-exited');
  showBanner('conversation-exited', 'Conversation is not running. Delete it or create a new conversation.');
  updateTerminalState();
}

function closeSocket() {
  window.clearTimeout(reconnectTimer);
  const activeSocket = socket;
  socket = null;
  if (activeSocket && activeSocket.readyState !== WebSocket.CLOSED) {
    activeSocket.close();
  }
}

async function refreshConversations() {
  try {
    await loadConversations();
  } catch {
    showBanner('error', 'Conversations are unavailable.');
  }
}

function startConversationPolling() {
  void refreshConversations();
  conversationTimer = window.setInterval(refreshConversations, conversationPollInterval);
}

createConversationForm.addEventListener('submit', (event) => {
  event.preventDefault();
  const name = conversationNameInput.value.trim();
  if (name === '') {
    conversationNameInput.setCustomValidity('Conversation name is required.');
    conversationNameInput.reportValidity();
    conversationNameInput.focus();
    return;
  }
  conversationNameInput.setCustomValidity('');

  void createConversation(name)
    .then(() => {
      closeConversationDialog();
    })
    .catch((error) => {
      conversationNameInput.setCustomValidity(error.message || 'Conversation could not be created.');
      conversationNameInput.reportValidity();
    });
});

newConversationButton.addEventListener('click', openConversationDialog);
conversationDialogClose.addEventListener('click', closeConversationDialog);
conversationCancelButton.addEventListener('click', closeConversationDialog);

window.addEventListener('popstate', () => {
  const nextActiveConversationId = chooseActiveConversationId({ preserveActive: false });
  if (nextActiveConversationId !== activeConversationId) {
    activeConversationId = nextActiveConversationId;
    resetTerminalForActiveConversation();
  }

  renderConversations();
  syncActiveConversation();
  updateConversationURL(activeConversationId);
});

terminal.onData((data) => {
  if (socket && socket.readyState === WebSocket.OPEN && !conversationExited) {
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
startConversationPolling();
