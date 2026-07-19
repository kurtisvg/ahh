const appShell = document.getElementById('app-shell');
const sidebar = document.getElementById('sidebar');
const sidebarBackdrop = document.getElementById('sidebar-backdrop');
const menuButton = document.getElementById('menu-button');
const conversationsModeButton = document.getElementById('conversations-mode-button');
const agentsModeButton = document.getElementById('agents-mode-button');
const conversationSidebar = document.getElementById('conversation-sidebar');
const agentSidebar = document.getElementById('agent-sidebar');
const newItemButton = document.getElementById('new-item-button');
const conversationListEl = document.getElementById('conversation-list');
const conversationEmptyEl = document.getElementById('conversation-empty');
const agentListEl = document.getElementById('agent-list');
const agentEmptyEl = document.getElementById('agent-empty');
const viewTitle = document.getElementById('view-title');
const viewSubtitle = document.getElementById('view-subtitle');
const statusSurface = document.getElementById('status-surface');
const statusEl = document.getElementById('status');
const statusText = document.getElementById('status-text');
const connectionBanner = document.getElementById('connection-banner');
const connectionBannerText = document.getElementById('connection-banner-text');
const terminalShell = document.getElementById('terminal-shell');
const terminalEl = document.getElementById('terminal');
const terminalState = document.getElementById('terminal-state');
const terminalStateText = document.getElementById('terminal-state-text');
const agentEditor = document.getElementById('agent-editor');
const agentEditorEmpty = document.getElementById('agent-editor-empty');
const agentEditorForm = document.getElementById('agent-editor-form');
const agentEditorName = document.getElementById('agent-editor-name');
const agentEditorHarness = document.getElementById('agent-editor-harness');
const agentSaveMessage = document.getElementById('agent-save-message');
const conversationDialog = document.getElementById('conversation-dialog');
const createConversationForm = document.getElementById('create-conversation-form');
const conversationDialogClose = document.getElementById('conversation-dialog-close');
const conversationCancelButton = document.getElementById('conversation-cancel-button');
const conversationNameInput = document.getElementById('conversation-name-input');
const conversationAgentSelect = document.getElementById('conversation-agent-select');
const agentDialog = document.getElementById('agent-dialog');
const createAgentForm = document.getElementById('create-agent-form');
const agentDialogClose = document.getElementById('agent-dialog-close');
const agentCancelButton = document.getElementById('agent-cancel-button');
const agentNameInput = document.getElementById('agent-name-input');
const agentHarnessSelect = document.getElementById('agent-harness-select');

const reconnectBaseDelay = 500;
const reconnectMaxDelay = 5000;
const conversationPollInterval = 3000;
const terminalStateMessages = {
  disconnected: 'Terminal disconnected',
  reconnecting: 'Reconnecting to terminal'
};

let conversations = [];
let agents = [];
let activeConversationId = '';
let activeAgentId = '';
let currentMode = 'conversations';
let socket;
let connectionState = 'disconnected';
let reconnectAttempt = 0;
let reconnectTimer;
let conversationTimer;
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
const decoder = new TextDecoder();

function appURL(path) {
  const basePath = appBasePath();
  return new URL(path, window.location.origin + basePath);
}

function appBasePath() {
  for (const marker of ['/conversations/', '/agents/']) {
    const markerIndex = window.location.pathname.lastIndexOf(marker);
    if (markerIndex >= 0) {
      return window.location.pathname.slice(0, markerIndex + 1);
    }
  }

  return window.location.pathname.endsWith('/')
    ? window.location.pathname
    : window.location.pathname + '/';
}

function routeFromPath() {
  const basePath = appBasePath();
  for (const mode of ['conversations', 'agents']) {
    const prefix = `${basePath}${mode}/`;
    if (!window.location.pathname.startsWith(prefix)) {
      continue;
    }
    const encodedId = window.location.pathname.slice(prefix.length).split('/')[0] || '';
    try {
      return { mode, id: decodeURIComponent(encodedId) };
    } catch {
      return { mode, id: '' };
    }
  }
  return { mode: 'conversations', id: '' };
}

function conversationIdFromPath() {
  const route = routeFromPath();
  return route.mode === 'conversations' ? route.id : '';
}

function updateSelectionURL(mode = 'replace') {
  if (!window.history || !window.history.replaceState) {
    return;
  }

  const url = new URL(window.location.href);
  const selectedId = currentMode === 'agents' ? activeAgentId : activeConversationId;
  const basePath = appBasePath();
  url.pathname = selectedId
    ? `${basePath}${currentMode}/${encodeURIComponent(selectedId)}`
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
  const url = appURL(`api/conversations/${encodeURIComponent(conversationId)}/tty`);
  url.protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return url.toString();
}

function activeConversation() {
  return conversations.find((conversation) => conversation.id === activeConversationId);
}

function activeAgent() {
  return agents.find((agent) => agent.id === activeAgentId);
}

function agentName(agentId) {
  return agents.find((agent) => agent.id === agentId)?.name || 'Agent unavailable';
}

function setStatus(state) {
  connectionState = state;
  statusEl.dataset.state = state;
  statusText.textContent = state;
  updateConversationStatuses();
  updateTerminalState();
}

function updateTerminalState() {
  if (hasTerminalOutput) {
    terminalState.hidden = true;
    return;
  }
  const message = activeConversation()
    ? terminalStateMessages[connectionState]
    : 'No conversation selected';
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

function renderConversations() {
  conversationListEl.replaceChildren();
  conversationEmptyEl.hidden = conversations.length !== 0;

  for (const conversation of conversations) {
    const stateName = conversation.id === activeConversationId ? connectionState : 'disconnected';
    const item = document.createElement('div');
    item.className = 'list-item conversation-item';
    item.dataset.conversationId = conversation.id;
    item.dataset.state = stateName;
    item.classList.toggle('is-active', conversation.id === activeConversationId);

    const selectButton = document.createElement('button');
    selectButton.className = 'item-select';
    selectButton.type = 'button';
    selectButton.ariaCurrent = conversation.id === activeConversationId ? 'page' : 'false';
    selectButton.addEventListener('click', () => selectConversation(conversation.id));
    selectButton.append(itemMain(conversation.name, `${agentName(conversation.agent_id)} · ${stateName}`));

    const deleteButton = document.createElement('button');
    deleteButton.className = 'conversation-delete';
    deleteButton.type = 'button';
    deleteButton.innerHTML = `
      <svg class="trash-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
        <path d="M3 6h18"></path><path d="M8 6V4h8v2"></path>
        <path d="M19 6l-1 14H6L5 6"></path><path d="M10 11v5"></path><path d="M14 11v5"></path>
      </svg>`;
    deleteButton.setAttribute('aria-label', `Delete ${conversation.name}`);
    deleteButton.addEventListener('click', () => void deleteConversation(conversation.id));

    const state = document.createElement('span');
    state.className = 'state-pill';
    state.ariaHidden = 'true';
    item.append(selectButton, deleteButton, state);
    conversationListEl.append(item);
  }
}

function renderAgents() {
  agentListEl.replaceChildren();
  agentEmptyEl.hidden = agents.length !== 0;
  for (const agent of agents) {
    const item = document.createElement('div');
    item.className = 'list-item agent-item';
    item.classList.toggle('is-active', agent.id === activeAgentId);

    const selectButton = document.createElement('button');
    selectButton.className = 'item-select';
    selectButton.type = 'button';
    selectButton.ariaCurrent = agent.id === activeAgentId ? 'page' : 'false';
    selectButton.addEventListener('click', () => selectAgent(agent.id));
    selectButton.append(itemMain(agent.name, agent.harness));
    item.append(selectButton);
    agentListEl.append(item);
  }
}

function itemMain(nameText, metaText) {
  const main = document.createElement('span');
  main.className = 'item-main';
  const name = document.createElement('span');
  name.className = 'item-name';
  name.textContent = nameText;
  const meta = document.createElement('span');
  meta.className = 'item-meta';
  meta.textContent = metaText;
  main.append(name, meta);
  return main;
}

function updateConversationStatuses() {
  for (const item of conversationListEl.querySelectorAll('.conversation-item')) {
    const conversation = conversations.find((candidate) => candidate.id === item.dataset.conversationId);
    const stateName = item.dataset.conversationId === activeConversationId ? connectionState : 'disconnected';
    item.dataset.state = stateName;
    item.querySelector('.item-meta').textContent = `${agentName(conversation?.agent_id)} · ${stateName}`;
  }
}

function renderAgentEditor() {
  const agent = activeAgent();
  agentEditorEmpty.hidden = Boolean(agent);
  agentEditorForm.hidden = !agent;
  if (!agent) {
    delete agentEditorForm.dataset.agentId;
    agentSaveMessage.textContent = '';
    return;
  }
  if (agentEditorForm.dataset.agentId !== agent.id) {
    agentEditorForm.dataset.agentId = agent.id;
    agentEditorName.value = agent.name;
    agentEditorName.setCustomValidity('');
    agentEditorHarness.value = agent.harness;
    agentSaveMessage.textContent = '';
  }
}

function updateViewHeading() {
  if (currentMode === 'agents') {
    const agent = activeAgent();
    viewTitle.textContent = agent?.name || 'Agents';
    viewSubtitle.textContent = agent ? 'Agent configuration' : 'No Agent selected';
    viewSubtitle.hidden = false;
    return;
  }
  const conversation = activeConversation();
  viewTitle.textContent = conversation?.name || 'Ahh';
  viewSubtitle.textContent = conversation ? agentName(conversation.agent_id) : 'No conversation selected';
  viewSubtitle.hidden = false;
}

function renderMode() {
  const conversationsActive = currentMode === 'conversations';
  appShell.dataset.mode = currentMode;
  conversationSidebar.hidden = !conversationsActive;
  agentSidebar.hidden = conversationsActive;
  terminalShell.hidden = !conversationsActive;
  agentEditor.hidden = conversationsActive;
  statusSurface.hidden = !conversationsActive;
  connectionBanner.hidden = !conversationsActive || connectionBanner.hidden;
  conversationsModeButton.classList.toggle('is-active', conversationsActive);
  conversationsModeButton.setAttribute('aria-selected', String(conversationsActive));
  agentsModeButton.classList.toggle('is-active', !conversationsActive);
  agentsModeButton.setAttribute('aria-selected', String(!conversationsActive));
  newItemButton.setAttribute('aria-label', conversationsActive ? 'New conversation' : 'New Agent');
  updateViewHeading();
  renderAgentEditor();
}

function setMode(mode, { history = 'push' } = {}) {
  if (mode !== 'agents' && mode !== 'conversations') {
    return;
  }
  const changed = currentMode !== mode;
  currentMode = mode;
  if (mode === 'agents') {
    closeSocket();
    setStatus('disconnected');
  } else if (changed) {
    resetTerminalForActiveConversation();
  }
  renderMode();
  if (mode === 'conversations') {
    syncActiveConversation();
    window.setTimeout(fitTerminal, 0);
  }
  updateSelectionURL(history);
  closeSidebar();
}

function selectConversation(conversationId) {
  const changed = conversationId !== activeConversationId;
  activeConversationId = conversationId;
  if (changed) {
    resetTerminalForActiveConversation();
  }
  currentMode = 'conversations';
  renderConversations();
  renderMode();
  syncActiveConversation();
  updateSelectionURL('push');
  closeSidebar();
}

function selectAgent(agentId) {
  activeAgentId = agentId;
  currentMode = 'agents';
  closeSocket();
  setStatus('disconnected');
  renderAgents();
  renderMode();
  updateSelectionURL('push');
  closeSidebar();
}

async function loadAgents({ preserveActive = true } = {}) {
  const response = await fetch(appURL('api/agents'), { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(`list Agents failed: ${response.status}`);
  }
  const payload = await response.json();
  agents = payload.agents || [];
  const route = routeFromPath();
  const requested = route.mode === 'agents' ? route.id : '';
  const activeStillExists = agents.some((agent) => agent.id === activeAgentId);
  if (requested && agents.some((agent) => agent.id === requested)) {
    activeAgentId = requested;
  } else if (!preserveActive || !activeStillExists) {
    activeAgentId = agents[0]?.id || '';
  }
  renderAgents();
  populateAgentPicker();
}

async function loadConversations({ preserveActive = true, syncConnection = true } = {}) {
  const response = await fetch(appURL('api/conversations'), { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(`list conversations failed: ${response.status}`);
  }
  const payload = await response.json();
  conversations = payload.conversations || [];
  const requested = conversationIdFromPath();
  const activeStillExists = conversations.some((conversation) => conversation.id === activeConversationId);
  let nextId = activeConversationId;
  if (requested && conversations.some((conversation) => conversation.id === requested)) {
    nextId = requested;
  } else if (!preserveActive || !activeStillExists) {
    nextId = conversations[0]?.id || '';
  }
  if (nextId !== activeConversationId) {
    activeConversationId = nextId;
    resetTerminalForActiveConversation();
  }
  renderConversations();
  if (syncConnection && currentMode === 'conversations') {
    syncActiveConversation();
  }
}

function populateAgentPicker() {
  const prior = conversationAgentSelect.value;
  conversationAgentSelect.replaceChildren();
  for (const agent of agents) {
    const option = document.createElement('option');
    option.value = agent.id;
    option.textContent = `${agent.name} (${agent.harness})`;
    conversationAgentSelect.append(option);
  }
  const preferred = agents.some((agent) => agent.id === prior)
    ? prior
    : agents.find((agent) => agent.id === 'claude-code')?.id || agents[0]?.id || '';
  conversationAgentSelect.value = preferred;
}

function resetTerminalForActiveConversation() {
  closeSocket();
  terminal.reset();
  hasTerminalOutput = false;
  reconnectAttempt = 0;
  hideBanner();
  setStatus('disconnected');
}

function syncActiveConversation() {
  if (currentMode !== 'conversations' || !activeConversation()) {
    closeSocket();
    setStatus('disconnected');
    updateViewHeading();
    return;
  }
  updateViewHeading();
  if (!socket || socket.readyState === WebSocket.CLOSED) {
    connectSocket();
  }
}

async function createConversation(name, agentId) {
  const response = await fetch(appURL('api/conversations'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, agent_id: agentId })
  });
  if (!response.ok) {
    throw new Error(await readErrorPayload(response) || `create conversation failed: ${response.status}`);
  }
  const conversation = await response.json();
  conversations = [conversation, ...conversations.filter((item) => item.id !== conversation.id)];
  activeConversationId = conversation.id;
  currentMode = 'conversations';
  resetTerminalForActiveConversation();
  renderConversations();
  renderMode();
  syncActiveConversation();
  updateSelectionURL('push');
}

async function deleteConversation(conversationId) {
  const response = await fetch(appURL(`api/conversations/${encodeURIComponent(conversationId)}`), { method: 'DELETE' });
  if (!response.ok && response.status !== 404) {
    showBanner('error', await readErrorPayload(response) || 'Conversation could not be deleted.');
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
  updateSelectionURL();
}

async function createAgent(name, harness) {
  const response = await fetch(appURL('api/agents'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, harness })
  });
  if (!response.ok) {
    throw new Error(await readErrorPayload(response) || `create Agent failed: ${response.status}`);
  }
  const agent = await response.json();
  agents = [...agents.filter((item) => item.id !== agent.id), agent]
    .sort((left, right) => left.name.localeCompare(right.name, undefined, { sensitivity: 'base' }));
  activeAgentId = agent.id;
  currentMode = 'agents';
  closeSocket();
  renderAgents();
  renderConversations();
  populateAgentPicker();
  renderMode();
  updateSelectionURL('push');
}

async function renameAgent(agentId, name) {
  const response = await fetch(appURL(`api/agents/${encodeURIComponent(agentId)}`), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name })
  });
  if (!response.ok) {
    throw new Error(await readErrorPayload(response) || `rename Agent failed: ${response.status}`);
  }
  const updated = await response.json();
  agents = agents.map((agent) => agent.id === updated.id ? updated : agent)
    .sort((left, right) => left.name.localeCompare(right.name, undefined, { sensitivity: 'base' }));
  renderAgents();
  renderConversations();
  populateAgentPicker();
  renderMode();
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
  const conversation = activeConversation();
  if (currentMode !== 'conversations' || !conversation) {
    return;
  }
  setStatus('reconnecting');
  socket = new WebSocket(terminalSocketURL(conversation.id));
  socket.binaryType = 'arraybuffer';
  const activeSocket = socket;
  activeSocket.addEventListener('open', () => {
    if (activeSocket !== socket) return;
    reconnectAttempt = 0;
    setStatus('connected');
    hideBanner();
    fitTerminal();
    terminal.focus();
  });
  activeSocket.addEventListener('message', (event) => {
    if (activeSocket !== socket) return;
    hasTerminalOutput = true;
    updateTerminalState();
    terminal.write(event.data instanceof ArrayBuffer ? decoder.decode(event.data, { stream: true }) : event.data);
  });
  activeSocket.addEventListener('close', () => {
    if (activeSocket === socket) void handleSocketClose();
  });
  activeSocket.addEventListener('error', () => {
    if (activeSocket === socket) setStatus('disconnected');
  });
}

async function handleSocketClose() {
  setStatus('disconnected');
  if (currentMode !== 'conversations') {
    return;
  }
  showBanner('disconnected', 'Terminal connection dropped. Checking conversation state.');
  try {
    await loadConversations({ syncConnection: false });
  } catch {
    scheduleReconnect();
    return;
  }
  if (activeConversation()) scheduleReconnect();
}

function scheduleReconnect() {
  if (currentMode !== 'conversations' || !activeConversation()) return;
  reconnectAttempt += 1;
  const delay = Math.min(reconnectBaseDelay * 2 ** (reconnectAttempt - 1), reconnectMaxDelay);
  setStatus('reconnecting');
  showBanner('reconnecting', `Terminal disconnected. Reconnecting in ${Math.ceil(delay / 1000)}s.`);
  window.clearTimeout(reconnectTimer);
  reconnectTimer = window.setTimeout(connectSocket, delay);
}

function closeSocket() {
  window.clearTimeout(reconnectTimer);
  const activeSocket = socket;
  socket = null;
  if (activeSocket && activeSocket.readyState !== WebSocket.CLOSED) activeSocket.close();
}

function fitTerminal() {
  if (currentMode !== 'conversations') return;
  fitAddon.fit();
  if (socket?.readyState === WebSocket.OPEN) {
    socket.send(JSON.stringify({ type: 'resize', rows: terminal.rows, cols: terminal.cols }));
  }
}

function openDialog(dialog, focusTarget) {
  hideBanner();
  if (typeof dialog.showModal === 'function') dialog.showModal();
  else dialog.setAttribute('open', '');
  focusTarget.focus();
}

function closeDialog(dialog) {
  if (typeof dialog.close === 'function') dialog.close();
  else dialog.removeAttribute('open');
}

function openConversationDialog() {
  conversationNameInput.value = '';
  conversationNameInput.setCustomValidity('');
  conversationAgentSelect.setCustomValidity('');
  populateAgentPicker();
  openDialog(conversationDialog, conversationNameInput);
}

function openAgentDialog() {
  agentNameInput.value = '';
  agentNameInput.setCustomValidity('');
  agentHarnessSelect.value = 'claude-code';
  openDialog(agentDialog, agentNameInput);
}

function openSidebar() {
  appShell.dataset.sidebarOpen = 'true';
  menuButton.setAttribute('aria-expanded', 'true');
  sidebarBackdrop.hidden = false;
}

function closeSidebar() {
  delete appShell.dataset.sidebarOpen;
  menuButton.setAttribute('aria-expanded', 'false');
  sidebarBackdrop.hidden = true;
}

async function refreshAll() {
  try {
    await Promise.all([loadAgents(), loadConversations({ syncConnection: false })]);
    renderConversations();
    renderMode();
    if (currentMode === 'conversations') syncActiveConversation();
    updateSelectionURL();
  } catch {
    showBanner('error', 'Ahh data is unavailable.');
  }
}

function startConversationPolling() {
  const initialRoute = routeFromPath();
  currentMode = initialRoute.mode;
  void refreshAll();
  conversationTimer = window.setInterval(refreshAll, conversationPollInterval);
}

createConversationForm.addEventListener('submit', (event) => {
  event.preventDefault();
  const name = conversationNameInput.value.trim();
  const agentId = conversationAgentSelect.value;
  if (!name || !agentId) {
    const control = name ? conversationAgentSelect : conversationNameInput;
    control.setCustomValidity(name ? 'An Agent is required.' : 'Conversation name is required.');
    control.reportValidity();
    control.focus();
    return;
  }
  conversationNameInput.setCustomValidity('');
  conversationAgentSelect.setCustomValidity('');
  void createConversation(name, agentId)
    .then(() => closeDialog(conversationDialog))
    .catch((error) => {
      conversationNameInput.setCustomValidity(error.message || 'Conversation could not be created.');
      conversationNameInput.reportValidity();
    });
});

createAgentForm.addEventListener('submit', (event) => {
  event.preventDefault();
  const name = agentNameInput.value.trim();
  if (!name) {
    agentNameInput.setCustomValidity('Agent name is required.');
    agentNameInput.reportValidity();
    return;
  }
  agentNameInput.setCustomValidity('');
  void createAgent(name, agentHarnessSelect.value)
    .then(() => closeDialog(agentDialog))
    .catch((error) => {
      agentNameInput.setCustomValidity(error.message || 'Agent could not be created.');
      agentNameInput.reportValidity();
    });
});

agentEditorForm.addEventListener('submit', (event) => {
  event.preventDefault();
  const agent = activeAgent();
  const name = agentEditorName.value.trim();
  if (!agent || !name) {
    agentEditorName.setCustomValidity('Agent name is required.');
    agentEditorName.reportValidity();
    return;
  }
  agentEditorName.setCustomValidity('');
  agentSaveMessage.textContent = 'Saving…';
  void renameAgent(agent.id, name)
    .then(() => {
      agentEditorName.value = activeAgent()?.name || name;
      agentSaveMessage.textContent = 'Saved';
    })
    .catch((error) => {
      agentSaveMessage.textContent = '';
      agentEditorName.setCustomValidity(error.message || 'Agent could not be saved.');
      agentEditorName.reportValidity();
    });
});

newItemButton.addEventListener('click', () => currentMode === 'agents' ? openAgentDialog() : openConversationDialog());
conversationsModeButton.addEventListener('click', () => setMode('conversations'));
agentsModeButton.addEventListener('click', () => setMode('agents'));
menuButton.addEventListener('click', () => appShell.dataset.sidebarOpen ? closeSidebar() : openSidebar());
sidebarBackdrop.addEventListener('click', closeSidebar);
conversationDialogClose.addEventListener('click', () => closeDialog(conversationDialog));
conversationCancelButton.addEventListener('click', () => closeDialog(conversationDialog));
agentDialogClose.addEventListener('click', () => closeDialog(agentDialog));
agentCancelButton.addEventListener('click', () => closeDialog(agentDialog));

window.addEventListener('popstate', () => {
  const route = routeFromPath();
  if (route.mode === 'agents' && agents.some((agent) => agent.id === route.id)) activeAgentId = route.id;
  if (route.mode === 'conversations' && conversations.some((conversation) => conversation.id === route.id)) activeConversationId = route.id;
  setMode(route.mode, { history: 'replace' });
  renderAgents();
  renderConversations();
});

terminal.onData((data) => {
  if (socket?.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ type: 'input', data }));
});

let resizeTimer;
window.addEventListener('resize', () => {
  window.clearTimeout(resizeTimer);
  resizeTimer = window.setTimeout(fitTerminal, 50);
  if (window.innerWidth > 760) closeSidebar();
});

fitTerminal();
startConversationPolling();
