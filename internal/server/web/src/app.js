import { renderShell } from './page-shell.js';
import { renderProject } from './project-page.js';
import { projectList } from './project-sidebar.js';
import { link } from './navigation.js';
import { mountTerminal } from './terminal.js';
import { startRouter, appURL, baseURL } from './router.js';

const app = document.querySelector('#app');
let activeView = () => {};
export const dispose = () => activeView();

const navigate = startRouter(async path => {
  dispose();
  const requests = new AbortController();
  let terminalDispose = () => {};
  activeView = () => { requests.abort(); terminalDispose(); };
  if (!path) { navigate('p', true); return; }
  const { sidebar, detail, projectsLink } = renderShell(app);
  document.querySelector('#terminal-link').href = appURL('terminal');

  if (path === 'terminal') {
    sidebar.textContent = 'Local terminal (startup directory)';
    detail.innerHTML = `
      <div class="toolbar">
        <p id="status" role="status">Loading…</p>
        <button id="start" hidden>Start terminal</button>
      </div>
      <button id="focus-terminal">Focus terminal</button>
      <div id="terminal" aria-label="Codex terminal"></div>
      <p id="notice" role="status"></p>
    `;
    detail.focus();
    terminalDispose = mountTerminal({
      container: detail.querySelector('#terminal'),
      status: detail.querySelector('#status'),
      start: detail.querySelector('#start'),
      notice: detail.querySelector('#notice'),
      baseURL,
    });
    detail.querySelector('#focus-terminal').onclick = () => detail.querySelector('.xterm-helper-textarea')?.focus();
    return;
  }
  detail.setAttribute('aria-live', 'polite');
  projectsLink.setAttribute('aria-current', 'page');
  const match = /^p(?:\/([^/]+))?$/.exec(path);
  if (!match) {
    detail.textContent = 'Page not found';
    detail.focus();
    return;
  }
  const id = match[1];
  sidebar.innerHTML = '<h2>Projects</h2><p>Loading Projects…</p>';
  detail.textContent = 'Loading…';
  // Keep focus on the content container while its asynchronous content changes.
  detail.focus();
  let projects;
  try {
    const response = await fetch(appURL('api/projects'), { cache: 'no-store', signal: requests.signal });
    if (!response.ok) throw new Error('Could not load Projects');
    projects = await response.json();
  } catch (error) {
    if (requests.signal.aborted) return;
    sidebar.innerHTML = '<h2>Projects</h2>';
    detail.textContent = error.message;
    const retry = document.createElement('button');
    retry.textContent = 'Retry';
    retry.onclick = () => navigate(path, true);
    detail.append(retry);
    return;
  }
  if (requests.signal.aborted) return;
  sidebar.innerHTML = '<h2>Projects</h2>';
  if (!id && projects.length) { navigate(`p/${projects[0].id}`, true); return; }
  if (!projects.length && !id) { detail.textContent = 'No Projects yet'; return; }
  const list = projectList(projects, id);
  sidebar.append(list);
  const project = projects.find(project => project.id === id);
  if (!project) {
    detail.replaceChildren(document.createTextNode('Project not found. '), link('p', 'Open Projects'));
    return;
  }
  renderProject({ detail, project });
});
