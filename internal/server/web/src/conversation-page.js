import { mountTerminal } from './terminal.js';
import { baseURL } from './router.js';
import { link } from './navigation.js';
import { icon } from './icons.js';

export function renderConversation({ detail, sidebar, project, refresh: onRefresh }) {
  const row = link(`p/${project.id}/c`, 'Conversation');
  row.setAttribute('aria-current', 'page');
  row.prepend(icon('conversations'));
  const nav = document.createElement('nav');
  nav.setAttribute('aria-label', 'Conversations');
  nav.append(row);
  sidebar.append(nav);
  if (project.state !== 'ready') {
    detail.innerHTML = `
      <div class="empty-state">
        <h2>Repository needs attention</h2>
        <p></p>
        <div class="actions"></div>
      </div>
    `;
    detail.querySelector('p').textContent = `Repository ${project.state}. Open settings for details.`;
    const settings = link(`p/${project.id}`, 'Open settings');
    settings.className = 'button';
    const refresh = document.createElement('button');
    refresh.textContent = 'Refresh';
    refresh.onclick = onRefresh;
    detail.querySelector('.actions').append(settings, refresh);
    return () => {};
  }
  detail.innerHTML = `
    <div class="conversation-heading">
      <div>
        <p class="eyebrow"></p>
        <h2>Conversation</h2>
      </div>
      <div class="toolbar">
        <p id="status" role="status">Loading…</p>
      </div>
    </div>
    <div class="conversation-start">
      <p>Start a conversation to work on this project.</p>
      <button id="start" class="primary" hidden>Start conversation</button>
      <p id="notice" role="status"></p>
    </div>
    <div id="terminal" aria-label="Conversation terminal" hidden></div>
  `;
  detail.querySelector('.eyebrow').textContent = project.name;
  const startPanel = detail.querySelector('.conversation-start');
  const terminal = detail.querySelector('#terminal');
  const start = detail.querySelector('#start');
  const notice = detail.querySelector('#notice');
  const onState = process => {
    const completed = process.exit_code != null || !!process.signal;
    const showTerminal = !!process.invocation && (completed || !['stopped', 'starting'].includes(process.state));
    terminal.hidden = !showTerminal;
    startPanel.hidden = showTerminal;
    const controls = showTerminal ? detail.querySelector('.toolbar') : startPanel;
    if (start.parentElement !== controls) controls.append(start);
    const messages = showTerminal ? detail : startPanel;
    if (notice.parentElement !== messages) messages.append(notice);
  };
  const dispose = mountTerminal({
    container: terminal,
    status: detail.querySelector('#status'),
    start,
    notice,
    baseURL,
    endpoint: `api/projects/${project.id}/conversations/default/terminal`,
    onMissing: onRefresh,
    onEscape: () => sidebar.querySelector('#project-select').focus(),
    onState
  });
  return dispose;
}
