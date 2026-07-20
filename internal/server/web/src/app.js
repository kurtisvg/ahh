const appShell = document.getElementById('app-shell');
const sidebar = document.getElementById('sidebar');
const sidebarBackdrop = document.getElementById('sidebar-backdrop');
const menuButton = document.getElementById('menu-button');
const projectsModeButton = document.getElementById('projects-mode-button');
const conversationsModeButton = document.getElementById('conversations-mode-button');
const agentsModeButton = document.getElementById('agents-mode-button');
const settingsModeButton = document.getElementById('settings-mode-button');
const projectSidebar = document.getElementById('project-sidebar');
const conversationSidebar = document.getElementById('conversation-sidebar');
const agentSidebar = document.getElementById('agent-sidebar');
const settingsSidebar = document.getElementById('settings-sidebar');
const sidebarModeTitle = document.getElementById('sidebar-mode-title');
const newItemButton = document.getElementById('new-item-button');
const projectListEl = document.getElementById('project-list');
const projectEmptyEl = document.getElementById('project-empty');
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
const retryConnectionButton = document.getElementById('retry-connection-button');
const stopRetryingButton = document.getElementById('stop-retrying-button');
const connectionDetailsButton = document.getElementById('connection-details-button');
const connectionDetails = document.getElementById('connection-details');
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
const projectEditor = document.getElementById('project-editor');
const projectEditorEmpty = document.getElementById('project-editor-empty');
const projectEditorForm = document.getElementById('project-editor-form');
const projectEditorTitle = document.getElementById('project-editor-title');
const projectEditorSource = document.getElementById('project-editor-source');
const projectStatusPill = document.getElementById('project-status-pill');
const projectDiagnostic = document.getElementById('project-diagnostic');
const projectDefaultBranch = document.getElementById('project-default-branch');
const projectRefreshButton = document.getElementById('project-refresh-button');
const projectDeleteButton = document.getElementById('project-delete-button');
const projectOpenConversationsButton = document.getElementById('project-open-conversations-button');
const projectEmptyCreateButton = document.getElementById('project-empty-create-button');
const settingsEditor = document.getElementById('settings-editor');
const settingsForm = document.getElementById('settings-form');
const authenticationMode = document.getElementById('authentication-mode');
const settingsSaveMessage = document.getElementById('settings-save-message');
const identityInvalidWarning = document.getElementById('identity-invalid-warning');
const sshPublicKey = document.getElementById('ssh-public-key');
const sshFingerprint = document.getElementById('ssh-fingerprint');
const copyPublicKeyButton = document.getElementById('copy-public-key-button');
const copyKeyMessage = document.getElementById('copy-key-message');
const regenerateIdentityButton = document.getElementById('regenerate-identity-button');
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
const projectDialog = document.getElementById('project-dialog');
const createProjectForm = document.getElementById('create-project-form');
const projectDialogClose = document.getElementById('project-dialog-close');
const projectCancelButton = document.getElementById('project-cancel-button');
const projectNameInput = document.getElementById('project-name-input');
const projectSourceType = document.getElementById('project-source-type');
const projectRepositoryInput = document.getElementById('project-repository-input');
const projectDeleteDialog = document.getElementById('project-delete-dialog');
const projectDeleteForm = document.getElementById('project-delete-form');
const projectDeleteDialogClose = document.getElementById('project-delete-dialog-close');
const projectDeleteCancelButton = document.getElementById('project-delete-cancel-button');
const projectDeleteWarning = document.getElementById('project-delete-warning');
const projectDeleteConfirmInput = document.getElementById('project-delete-confirm-input');
const regenerateDialog = document.getElementById('regenerate-dialog');
const regenerateForm = document.getElementById('regenerate-form');
const regenerateDialogClose = document.getElementById('regenerate-dialog-close');
const regenerateCancelButton = document.getElementById('regenerate-cancel-button');
const regenerateConfirmInput = document.getElementById('regenerate-confirm-input');

const reconnectDelays = [1000, 2000, 4000, 8000, 15000];
const stableConnectionDelay = 3000;
const conversationPollInterval = 3000;
const terminalStateMessages = {
  disconnected: 'Terminal disconnected',
  reconnecting: 'Reconnecting to terminal'
};

let conversations = [];
let agents = [];
let projects = [];
let settings;
let activeConversationId = '';
let activeAgentId = '';
let activeProjectId = '';
let currentMode = 'projects';
let socket;
let connectionState = 'disconnected';
let reconnectAttempt = 0;
let reconnectTimer;
let stableConnectionTimer;
let automaticReconnect = true;
let connectionRecoveryActive = false;
let lastDisconnectMessage = 'Connection interrupted.';
let lastDisconnectDetail = '';
let pausedReconnectMessage = '';
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
  for (const marker of ['/projects/', '/conversations/', '/agents/']) {
    const markerIndex = window.location.pathname.lastIndexOf(marker);
    if (markerIndex >= 0) {
      return window.location.pathname.slice(0, markerIndex + 1);
    }
  }

  const settingsMarker = '/settings';
  const settingsIndex = window.location.pathname.lastIndexOf(settingsMarker);
  if (settingsIndex >= 0 && settingsIndex + settingsMarker.length === window.location.pathname.length) {
    return window.location.pathname.slice(0, settingsIndex + 1);
  }

  return window.location.pathname.endsWith('/')
    ? window.location.pathname
    : window.location.pathname + '/';
}

function routeFromPath() {
  const basePath = appBasePath();
  if (window.location.pathname === `${basePath}settings`) {
    return { mode: 'settings', id: '' };
  }
  for (const mode of ['projects', 'conversations', 'agents']) {
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
  return { mode: 'projects', id: '' };
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
  const selectedId = currentMode === 'projects'
    ? activeProjectId
    : currentMode === 'agents'
      ? activeAgentId
      : activeConversationId;
  const basePath = appBasePath();
  url.pathname = currentMode === 'settings'
    ? `${basePath}settings`
    : selectedId
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

function activeProject() {
  return projects.find((project) => project.id === activeProjectId);
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

function showBanner(state, message, {
  canRetry = false,
  canStop = false,
  detail = '',
  source = 'connection'
} = {}) {
  connectionBanner.dataset.state = state;
  connectionBanner.dataset.source = source;
  connectionBannerText.textContent = message;
  retryConnectionButton.hidden = !canRetry;
  stopRetryingButton.hidden = !canStop;
  connectionDetailsButton.hidden = !detail;
  connectionDetailsButton.setAttribute('aria-expanded', 'false');
  connectionDetails.textContent = detail;
  connectionDetails.hidden = true;
  connectionBanner.hidden = false;
}

function hideBanner() {
  connectionBanner.hidden = true;
  delete connectionBanner.dataset.source;
  retryConnectionButton.hidden = true;
  stopRetryingButton.hidden = true;
  connectionDetailsButton.hidden = true;
  connectionDetailsButton.setAttribute('aria-expanded', 'false');
  connectionDetails.hidden = true;
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

function renderProjects() {
  projectListEl.replaceChildren();
  projectEmptyEl.hidden = projects.length !== 0;
  for (const project of projects) {
    const item = document.createElement('div');
    item.className = 'list-item project-item';
    item.dataset.state = project.status;
    item.classList.toggle('is-active', project.id === activeProjectId);

    const selectButton = document.createElement('button');
    selectButton.className = 'item-select';
    selectButton.type = 'button';
    selectButton.ariaCurrent = project.id === activeProjectId ? 'page' : 'false';
    selectButton.addEventListener('click', () => selectProject(project.id));
    selectButton.append(itemMain(project.name, `${project.source.repository} · ${project.status}`));

    const state = document.createElement('span');
    state.className = 'state-pill';
    state.ariaHidden = 'true';
    item.append(selectButton, state);
    projectListEl.append(item);
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

async function renderProjectEditor() {
  const project = activeProject();
  projectEditorEmpty.hidden = Boolean(project);
  projectEditorForm.hidden = !project;
  if (!project) return;

  projectEditorTitle.textContent = project.name;
  projectEditorSource.textContent = `GitHub · ${project.source.repository}`;
  projectStatusPill.textContent = project.status;
  projectStatusPill.dataset.state = project.status;
  projectDiagnostic.textContent = project.diagnostic || '';
  projectDiagnostic.hidden = !project.diagnostic;
  projectDefaultBranch.replaceChildren();
  try {
    const response = await fetch(appURL(`api/projects/${encodeURIComponent(project.id)}/branches`), { cache: 'no-store' });
    if (!response.ok) throw new Error(await readErrorPayload(response) || 'Branches are unavailable.');
    const payload = await response.json();
    if (activeProjectId !== project.id) return;
    for (const branch of payload.branches || []) {
      const option = document.createElement('option');
      option.value = `${branch.kind}:${branch.name}`;
      option.textContent = `${branch.kind === 'remote' ? 'origin/' : ''}${branch.name}`;
      projectDefaultBranch.append(option);
    }
    projectDefaultBranch.value = `${project.default_branch.kind}:${project.default_branch.name}`;
  } catch (error) {
    const option = document.createElement('option');
    option.value = `${project.default_branch.kind}:${project.default_branch.name}`;
    option.textContent = `${project.default_branch.kind === 'remote' ? 'origin/' : ''}${project.default_branch.name}`;
    projectDefaultBranch.append(option);
    projectDiagnostic.textContent = project.diagnostic || error.message;
    projectDiagnostic.hidden = false;
  }
}

function renderSettings() {
  if (!settings) return;
  authenticationMode.value = settings.authentication_mode;
  const identity = settings.ssh_identity || {};
  identityInvalidWarning.hidden = identity.status !== 'invalid';
  sshPublicKey.value = identity.public_key || '';
  sshFingerprint.value = identity.fingerprint || '';
  copyPublicKeyButton.disabled = !identity.public_key;
  regenerateIdentityButton.disabled = !identity.fingerprint;
}

function updateViewHeading() {
  if (currentMode === 'projects') {
    const project = activeProject();
    viewTitle.textContent = project?.name || 'Projects';
    viewSubtitle.textContent = project ? project.source.repository : 'Register a GitHub repository';
    viewSubtitle.hidden = false;
    return;
  }
  if (currentMode === 'settings') {
    viewTitle.textContent = 'Settings';
    viewSubtitle.textContent = 'Authentication and installation identity';
    viewSubtitle.hidden = false;
    return;
  }
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
  const agentsActive = currentMode === 'agents';
  const projectsActive = currentMode === 'projects';
  const settingsActive = currentMode === 'settings';
  appShell.dataset.mode = currentMode;
  projectSidebar.hidden = !projectsActive;
  conversationSidebar.hidden = !conversationsActive;
  agentSidebar.hidden = !agentsActive;
  settingsSidebar.hidden = !settingsActive;
  terminalShell.hidden = !conversationsActive;
  agentEditor.hidden = !agentsActive;
  projectEditor.hidden = !projectsActive;
  settingsEditor.hidden = !settingsActive;
  statusSurface.hidden = !conversationsActive;
  connectionBanner.hidden = !conversationsActive || connectionBanner.hidden;
  projectsModeButton.classList.toggle('is-active', projectsActive);
  projectsModeButton.setAttribute('aria-current', projectsActive ? 'page' : 'false');
  conversationsModeButton.classList.toggle('is-active', conversationsActive);
  conversationsModeButton.setAttribute('aria-current', conversationsActive ? 'page' : 'false');
  agentsModeButton.classList.toggle('is-active', agentsActive);
  agentsModeButton.setAttribute('aria-current', agentsActive ? 'page' : 'false');
  settingsModeButton.classList.toggle('is-active', settingsActive);
  settingsModeButton.setAttribute('aria-current', settingsActive ? 'page' : 'false');
  const modeLabels = { projects: 'Projects', agents: 'Agents', conversations: 'Conversations', settings: 'Settings' };
  sidebarModeTitle.textContent = modeLabels[currentMode];
  newItemButton.hidden = settingsActive;
  newItemButton.setAttribute('aria-label', projectsActive ? 'New Project' : agentsActive ? 'New Agent' : 'New conversation');
  updateViewHeading();
  renderAgentEditor();
  if (projectsActive) void renderProjectEditor();
  if (settingsActive) renderSettings();
}

function setMode(mode, { history = 'push' } = {}) {
  if (!['projects', 'agents', 'conversations', 'settings'].includes(mode)) {
    return;
  }
  const changed = currentMode !== mode;
  currentMode = mode;
  if (mode !== 'conversations') {
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

function selectProject(projectId) {
  activeProjectId = projectId;
  currentMode = 'projects';
  closeSocket();
  setStatus('disconnected');
  renderProjects();
  renderMode();
  updateSelectionURL('push');
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
    throw new Error(`Agents could not be loaded (server returned ${response.status}).`);
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

async function loadProjects({ preserveActive = true } = {}) {
  const response = await fetch(appURL('api/projects'), { cache: 'no-store' });
  if (!response.ok) throw new Error(`Projects could not be loaded (server returned ${response.status}).`);
  const payload = await response.json();
  projects = payload.projects || [];
  const route = routeFromPath();
  const requested = route.mode === 'projects' ? route.id : '';
  const activeStillExists = projects.some((project) => project.id === activeProjectId);
  if (requested && projects.some((project) => project.id === requested)) {
    activeProjectId = requested;
  } else if (!preserveActive || !activeStillExists) {
    activeProjectId = projects[0]?.id || '';
  }
  renderProjects();
}

async function loadSettings() {
  const response = await fetch(appURL('api/settings'), { cache: 'no-store' });
  if (!response.ok) throw new Error(`Settings could not be loaded (server returned ${response.status}).`);
  settings = await response.json();
  renderSettings();
}

async function loadConversations({ preserveActive = true, syncConnection = true } = {}) {
  const response = await fetch(appURL('api/conversations'), { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(`Conversations could not be loaded (server returned ${response.status}).`);
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
    option.textContent = agent.name;
    conversationAgentSelect.append(option);
  }
  const preferred = agents.some((agent) => agent.id === prior)
    ? prior
    : agents.find((agent) => agent.name.toLowerCase() === 'default')?.id || agents[0]?.id || '';
  conversationAgentSelect.value = preferred;
}

function resetTerminalForActiveConversation() {
  closeSocket();
  terminal.reset();
  hasTerminalOutput = false;
  reconnectAttempt = 0;
  automaticReconnect = true;
  lastDisconnectMessage = 'Connection interrupted.';
  lastDisconnectDetail = '';
  pausedReconnectMessage = '';
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
  if (!socket && !reconnectTimer && !connectionRecoveryActive && automaticReconnect) {
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

async function createProject(name, sourceType, repository) {
  const response = await fetch(appURL('api/projects'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, source: { type: sourceType, repository } })
  });
  if (!response.ok) throw new Error(await readErrorPayload(response) || `create Project failed: ${response.status}`);
  const project = await response.json();
  projects = [...projects.filter((item) => item.id !== project.id), project]
    .sort((left, right) => left.name.localeCompare(right.name, undefined, { sensitivity: 'base' }));
  activeProjectId = project.id;
  currentMode = 'projects';
  renderProjects();
  renderMode();
  updateSelectionURL('push');
}

async function saveProjectDefaultBranch(projectId) {
  const [kind, ...nameParts] = projectDefaultBranch.value.split(':');
  const response = await fetch(appURL(`api/projects/${encodeURIComponent(projectId)}`), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ default_branch: { kind, name: nameParts.join(':') } })
  });
  if (!response.ok) throw new Error(await readErrorPayload(response) || 'Project could not be saved.');
  const updated = await response.json();
  projects = projects.map((project) => project.id === updated.id ? updated : project);
  renderProjects();
  await renderProjectEditor();
}

async function refreshProject(projectId) {
  projectRefreshButton.disabled = true;
  try {
    const response = await fetch(appURL(`api/projects/${encodeURIComponent(projectId)}/fetch`), { method: 'POST' });
    if (!response.ok) {
      await loadProjects();
      throw new Error(await readErrorPayload(response) || 'Project refresh failed.');
    }
    const updated = await response.json();
    projects = projects.map((project) => project.id === updated.id ? updated : project);
    renderProjects();
    await renderProjectEditor();
  } finally {
    projectRefreshButton.disabled = false;
  }
}

async function deleteProject(projectId) {
  const response = await fetch(appURL(`api/projects/${encodeURIComponent(projectId)}`), { method: 'DELETE' });
  if (!response.ok && response.status !== 404) throw new Error(await readErrorPayload(response) || 'Project could not be deleted.');
  projects = projects.filter((project) => project.id !== projectId);
  activeProjectId = projects[0]?.id || '';
  renderProjects();
  renderMode();
  updateSelectionURL();
}

async function saveSettings(mode) {
  const response = await fetch(appURL('api/settings'), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ authentication_mode: mode })
  });
  if (!response.ok) throw new Error(await readErrorPayload(response) || 'Settings could not be saved.');
  settings = await response.json();
  renderSettings();
}

async function regenerateIdentity(confirmFingerprint) {
  const response = await fetch(appURL('api/settings/ssh-identity/regenerate'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ confirm_fingerprint: confirmFingerprint })
  });
  if (!response.ok) throw new Error(await readErrorPayload(response) || 'SSH identity could not be regenerated.');
  settings = await response.json();
  renderSettings();
}

async function createAgent(name) {
  const response = await fetch(appURL('api/agents'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, harness: 'claude-code' })
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
  if (currentMode !== 'conversations' || !conversation || socket) {
    return;
  }

  window.clearTimeout(reconnectTimer);
  reconnectTimer = undefined;
  setStatus('reconnecting');
  showBanner(
    'reconnecting',
    reconnectAttempt === 0 ? 'Connecting to terminal…' : 'Retrying terminal connection…',
    { canStop: reconnectAttempt > 0, detail: lastDisconnectDetail }
  );
  socket = new WebSocket(terminalSocketURL(conversation.id));
  socket.binaryType = 'arraybuffer';
  const activeSocket = socket;
  activeSocket.addEventListener('open', () => {
    if (activeSocket !== socket) return;
    setStatus('connected');
    showBanner('reconnecting', 'Connected. Waiting for terminal…', { detail: lastDisconnectDetail });
    window.clearTimeout(stableConnectionTimer);
    stableConnectionTimer = window.setTimeout(markConnectionStable, stableConnectionDelay);
    fitTerminal();
    terminal.focus();
  });
  activeSocket.addEventListener('message', (event) => {
    if (activeSocket !== socket) return;
    markConnectionStable();
    hasTerminalOutput = true;
    updateTerminalState();
    terminal.write(event.data instanceof ArrayBuffer ? decoder.decode(event.data, { stream: true }) : event.data);
  });
  activeSocket.addEventListener('close', (event) => {
    if (activeSocket !== socket) return;
    socket = null;
    window.clearTimeout(stableConnectionTimer);
    stableConnectionTimer = undefined;
    void handleSocketClose(event, conversation.id);
  });
  activeSocket.addEventListener('error', () => {
    if (activeSocket === socket) setStatus('disconnected');
  });
}

function markConnectionStable() {
  if (!socket || socket.readyState !== WebSocket.OPEN) return;
  window.clearTimeout(stableConnectionTimer);
  stableConnectionTimer = undefined;
  reconnectAttempt = 0;
  automaticReconnect = true;
  connectionRecoveryActive = false;
  lastDisconnectMessage = 'Connection interrupted.';
  lastDisconnectDetail = '';
  pausedReconnectMessage = '';
  setStatus('connected');
  hideBanner();
}

function describeSocketClose(event) {
  const detail = `WebSocket closed with code ${event.code}${event.reason ? `: ${event.reason}` : ''}.`;
  switch (event.reason) {
    case 'conversation unavailable':
      return { message: 'Agent or configuration unavailable.', detail, retryAutomatically: false };
    case 'wrapper unavailable':
      return { message: 'Terminal wrapper is unreachable.', detail, retryAutomatically: true };
    case 'terminal proxy failed':
      return { message: 'Terminal connection was interrupted.', detail, retryAutomatically: true };
  }
  if (event.code === 1000) {
    return { message: 'Claude Code exited or the terminal closed.', detail, retryAutomatically: false };
  }
  return { message: 'Terminal connection was interrupted.', detail, retryAutomatically: true };
}

async function handleSocketClose(event, conversationId) {
  setStatus('disconnected');
  if (currentMode !== 'conversations' || activeConversationId !== conversationId) {
    return;
  }

  connectionRecoveryActive = true;

  const description = describeSocketClose(event);
  lastDisconnectMessage = description.message;
  lastDisconnectDetail = description.detail;
  if (!description.retryAutomatically) {
    pauseAutomaticReconnect(description.message);
    return;
  }

  showBanner('disconnected', `${description.message} Checking conversation state…`, {
    canRetry: true,
    canStop: true,
    detail: description.detail
  });
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
  if (!automaticReconnect) {
    pauseAutomaticReconnect(lastDisconnectMessage);
    return;
  }
  if (reconnectAttempt >= reconnectDelays.length) {
    pauseAutomaticReconnect(`Automatic retries paused after ${reconnectDelays.length} attempts.`);
    return;
  }

  const delay = reconnectDelays[reconnectAttempt];
  reconnectAttempt += 1;
  setStatus('reconnecting');
  showBanner(
    'reconnecting',
    `${lastDisconnectMessage} Retrying in ${Math.ceil(delay / 1000)}s (attempt ${reconnectAttempt}/${reconnectDelays.length}).`,
    { canRetry: true, canStop: true, detail: lastDisconnectDetail }
  );
  window.clearTimeout(reconnectTimer);
  reconnectTimer = window.setTimeout(() => {
    reconnectTimer = undefined;
    connectSocket();
  }, delay);
}

function pauseAutomaticReconnect(message) {
  automaticReconnect = false;
  connectionRecoveryActive = false;
  pausedReconnectMessage = message;
  window.clearTimeout(reconnectTimer);
  reconnectTimer = undefined;
  setStatus('disconnected');
  showPausedReconnect();
}

function showPausedReconnect() {
  showBanner('disconnected', pausedReconnectMessage || lastDisconnectMessage, {
    canRetry: true,
    detail: lastDisconnectDetail
  });
}

function retryConnectionNow() {
  closeSocket();
  reconnectAttempt = 0;
  automaticReconnect = true;
  pausedReconnectMessage = '';
  connectSocket();
}

function stopAutomaticReconnect() {
  pauseAutomaticReconnect(`${lastDisconnectMessage} Automatic retries stopped.`);
}

function closeSocket() {
  window.clearTimeout(reconnectTimer);
  reconnectTimer = undefined;
  window.clearTimeout(stableConnectionTimer);
  stableConnectionTimer = undefined;
  connectionRecoveryActive = false;
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
  openDialog(agentDialog, agentNameInput);
}

function openProjectDialog() {
  projectNameInput.value = '';
  projectSourceType.value = 'github';
  projectRepositoryInput.value = '';
  projectNameInput.setCustomValidity('');
  projectRepositoryInput.setCustomValidity('');
  openDialog(projectDialog, projectNameInput);
}

function openProjectDeleteDialog() {
  const project = activeProject();
  if (!project) return;
  projectDeleteConfirmInput.value = '';
  projectDeleteConfirmInput.setCustomValidity('');
  projectDeleteWarning.textContent = `Deleting ${project.name} removes its managed repository. This cannot be undone.`;
  openDialog(projectDeleteDialog, projectDeleteConfirmInput);
}

function openRegenerateDialog() {
  regenerateConfirmInput.value = '';
  regenerateConfirmInput.setCustomValidity('');
  openDialog(regenerateDialog, regenerateConfirmInput);
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
    await Promise.all([loadProjects(), loadAgents(), loadConversations({ syncConnection: false }), loadSettings()]);
    if (connectionBanner.dataset.source === 'data') {
      const terminalRetryPaused = currentMode === 'conversations' &&
        activeConversation() &&
        !socket &&
        !automaticReconnect;
      if (terminalRetryPaused) {
        showPausedReconnect();
      } else {
        hideBanner();
      }
    }
    renderConversations();
    renderMode();
    if (currentMode === 'conversations') syncActiveConversation();
    updateSelectionURL();
  } catch (error) {
    const message = error instanceof TypeError
      ? 'Cannot reach the Ahh server. Check that it is running, then refresh this page.'
      : error?.message || 'Projects, Agents, Conversations, and Settings could not be loaded.';
    showBanner('error', message, { source: 'data' });
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
  void createAgent(name)
    .then(() => closeDialog(agentDialog))
    .catch((error) => {
      agentNameInput.setCustomValidity(error.message || 'Agent could not be created.');
      agentNameInput.reportValidity();
    });
});

createProjectForm.addEventListener('submit', (event) => {
  event.preventDefault();
  const name = projectNameInput.value;
  const sourceType = projectSourceType.value;
  const repository = projectRepositoryInput.value.trim();
  if (!name || !sourceType || !repository) {
    const control = !name ? projectNameInput : !sourceType ? projectSourceType : projectRepositoryInput;
    control.setCustomValidity(!name ? 'Project name is required.' : !sourceType ? 'A source type is required.' : 'GitHub owner/repository is required.');
    control.reportValidity();
    return;
  }
  projectNameInput.setCustomValidity('');
  projectSourceType.setCustomValidity('');
  projectRepositoryInput.setCustomValidity('');
  void createProject(name, sourceType, repository)
    .then(() => closeDialog(projectDialog))
    .catch((error) => {
      projectRepositoryInput.setCustomValidity(error.message || 'Project could not be created.');
      projectRepositoryInput.reportValidity();
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

projectEditorForm.addEventListener('submit', (event) => {
  event.preventDefault();
  const project = activeProject();
  if (!project) return;
  void saveProjectDefaultBranch(project.id).catch((error) => {
    projectDefaultBranch.setCustomValidity(error.message || 'Project could not be saved.');
    projectDefaultBranch.reportValidity();
  });
});

settingsForm.addEventListener('submit', (event) => {
  event.preventDefault();
  settingsSaveMessage.textContent = 'Saving…';
  void saveSettings(authenticationMode.value)
    .then(() => { settingsSaveMessage.textContent = 'Saved. Restart existing Conversations to apply the mode.'; })
    .catch((error) => { settingsSaveMessage.textContent = error.message || 'Settings could not be saved.'; });
});

projectDeleteForm.addEventListener('submit', (event) => {
  event.preventDefault();
  const project = activeProject();
  if (!project || projectDeleteConfirmInput.value !== project.name) {
    projectDeleteConfirmInput.setCustomValidity('Type the exact Project name.');
    projectDeleteConfirmInput.reportValidity();
    return;
  }
  projectDeleteConfirmInput.setCustomValidity('');
  void deleteProject(project.id)
    .then(() => closeDialog(projectDeleteDialog))
    .catch((error) => {
      projectDeleteConfirmInput.setCustomValidity(error.message || 'Project could not be deleted.');
      projectDeleteConfirmInput.reportValidity();
    });
});

regenerateForm.addEventListener('submit', (event) => {
  event.preventDefault();
  const fingerprint = settings?.ssh_identity?.fingerprint || '';
  if (!fingerprint || regenerateConfirmInput.value !== fingerprint) {
    regenerateConfirmInput.setCustomValidity('Type the exact current fingerprint.');
    regenerateConfirmInput.reportValidity();
    return;
  }
  regenerateConfirmInput.setCustomValidity('');
  void regenerateIdentity(fingerprint)
    .then(() => closeDialog(regenerateDialog))
    .catch((error) => {
      regenerateConfirmInput.setCustomValidity(error.message || 'Identity could not be regenerated.');
      regenerateConfirmInput.reportValidity();
    });
});

newItemButton.addEventListener('click', () => {
  if (currentMode === 'projects') openProjectDialog();
  else if (currentMode === 'agents') openAgentDialog();
  else openConversationDialog();
});
projectsModeButton.addEventListener('click', () => setMode('projects'));
conversationsModeButton.addEventListener('click', () => setMode('conversations'));
agentsModeButton.addEventListener('click', () => setMode('agents'));
settingsModeButton.addEventListener('click', () => setMode('settings'));
projectEmptyCreateButton.addEventListener('click', openProjectDialog);
projectRefreshButton.addEventListener('click', () => {
  const project = activeProject();
  if (project) void refreshProject(project.id).catch((error) => {
    projectDiagnostic.textContent = error.message || 'Project refresh failed.';
    projectDiagnostic.hidden = false;
  });
});
projectDeleteButton.addEventListener('click', openProjectDeleteDialog);
projectOpenConversationsButton.addEventListener('click', () => setMode('conversations'));
copyPublicKeyButton.addEventListener('click', () => {
  if (!navigator.clipboard?.writeText) {
    copyKeyMessage.textContent = 'Clipboard access is unavailable; select the key manually.';
    return;
  }
  void navigator.clipboard.writeText(sshPublicKey.value)
    .then(() => { copyKeyMessage.textContent = 'Copied'; })
    .catch(() => { copyKeyMessage.textContent = 'Copy failed; select the key manually.'; });
});
regenerateIdentityButton.addEventListener('click', openRegenerateDialog);
menuButton.addEventListener('click', () => appShell.dataset.sidebarOpen ? closeSidebar() : openSidebar());
sidebarBackdrop.addEventListener('click', closeSidebar);
conversationDialogClose.addEventListener('click', () => closeDialog(conversationDialog));
conversationCancelButton.addEventListener('click', () => closeDialog(conversationDialog));
agentDialogClose.addEventListener('click', () => closeDialog(agentDialog));
agentCancelButton.addEventListener('click', () => closeDialog(agentDialog));
projectDialogClose.addEventListener('click', () => closeDialog(projectDialog));
projectCancelButton.addEventListener('click', () => closeDialog(projectDialog));
projectDeleteDialogClose.addEventListener('click', () => closeDialog(projectDeleteDialog));
projectDeleteCancelButton.addEventListener('click', () => closeDialog(projectDeleteDialog));
regenerateDialogClose.addEventListener('click', () => closeDialog(regenerateDialog));
regenerateCancelButton.addEventListener('click', () => closeDialog(regenerateDialog));
retryConnectionButton.addEventListener('click', retryConnectionNow);
stopRetryingButton.addEventListener('click', stopAutomaticReconnect);
connectionDetailsButton.addEventListener('click', () => {
  const expanded = connectionDetailsButton.getAttribute('aria-expanded') === 'true';
  connectionDetailsButton.setAttribute('aria-expanded', String(!expanded));
  connectionDetails.hidden = expanded;
});

window.addEventListener('popstate', () => {
  const route = routeFromPath();
  if (route.mode === 'projects' && projects.some((project) => project.id === route.id)) activeProjectId = route.id;
  if (route.mode === 'agents' && agents.some((agent) => agent.id === route.id)) activeAgentId = route.id;
  const conversationExists = route.mode === 'conversations' &&
    conversations.some((conversation) => conversation.id === route.id);
  const conversationChanged = conversationExists && activeConversationId !== route.id;
  if (conversationExists) activeConversationId = route.id;
  if (conversationChanged && currentMode === 'conversations') resetTerminalForActiveConversation();
  setMode(route.mode, { history: 'replace' });
  renderProjects();
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
