import { renderShell } from './page-shell.js';
import { renderProject } from './project-page.js';
import { projectList, renderProjectPicker } from './project-sidebar.js';
import { link } from './navigation.js';
import { renderConversation } from './conversation-page.js';
import { icon } from './icons.js';
import { addProject } from './project-create.js';
import { startRouter, appURL } from './router.js';

const app = document.querySelector('#app');
let activeView = () => {};
let focusProjects = false;
export const dispose = () => activeView();

const navigate = startRouter(async path => {
  dispose();
  const requests = new AbortController();
  let terminalDispose = () => {};
  activeView = () => { requests.abort(); terminalDispose(); };
  if (!path) { navigate('p', true); return; }
  const match = /^p(?:\/([^/]+))?(\/c)?$/.exec(path);
  const id = match?.[1];
  const conversations = path === 'c' || !!match?.[2];
  document.title = `${conversations ? 'Conversations' : 'Settings'} · Ahh`;
  const { sidebar, detail, projectsLink, conversationsLink } = renderShell(app, {
    id,
    conversations,
  });
  detail.tabIndex = -1;
  detail.focus();
  (conversations ? conversationsLink : projectsLink).setAttribute('aria-current', 'page');
  if (!match && !conversations) { document.title = 'Page not found · Ahh'; detail.textContent = 'Page not found'; return; }
  sidebar.innerHTML = '<div class="sidebar-heading"><h2>Projects</h2></div><p>Loading Projects…</p>';
  detail.textContent = 'Loading…';
  let projects;
  try {
    const response = await fetch(appURL('api/projects'), { cache: 'no-store', signal: requests.signal });
    if (!response.ok) throw new Error('Could not load Projects');
    projects = await response.json();
  } catch (error) {
    if (requests.signal.aborted) return;
    sidebar.innerHTML = '<div class="sidebar-heading"><h2 tabindex="-1">Projects</h2></div>';
    detail.innerHTML = '<div class="empty-state"><h2>Could not load Projects</h2><p role="alert"></p></div>';
    const empty = detail.firstElementChild;
    empty.querySelector('p').textContent = 'Check your connection and try again.';
    const retry = document.createElement('button');
    retry.textContent = 'Retry';
    retry.onclick = () => navigate(path, true);
    empty.append(retry);
    return;
  }
  if (requests.signal.aborted) return;
  sidebar.innerHTML = '<div class="sidebar-heading"><h2 tabindex="-1">Projects</h2></div>';
  if (!conversations) addProject(sidebar, requests.signal, navigate);
  if (!id && projects.length) { navigate(`p/${projects[0].id}${conversations ? "/c" : ""}`, true); return; }
  const key = id;
  const project = projects.find(project => project.id === key);
  if (focusProjects) {
    const heading = sidebar.querySelector('h2');
    heading.tabIndex = -1;
    heading.focus();
    focusProjects = false;
  }
  const list = projectList(projects, project?.id);
  if (conversations) renderProjectPicker(sidebar, projects, project?.id, navigate); else sidebar.append(list);
  if (!projects.length && !id) {
    detail.innerHTML = '<div class="empty-state"><h2>No Projects yet</h2><p></p></div>';
    const empty = detail.firstElementChild;
    empty.prepend(icon('folder'));
    empty.querySelector('p').textContent = conversations ? 'Create a Project to start a conversation.' : 'Use the + beside Projects to add your first repository.';
    if (conversations) { const open = link('p', 'Open Projects'); open.className = 'button'; empty.append(open); }
    return;
  }
  if (!project) {
    document.title = 'Project not found · Ahh';
    detail.innerHTML = '<div class="empty-state"><h2>Project not found</h2><p>This Project may have been removed.</p></div>';
    const open = link('p', 'Open Projects');
    open.className = 'button';
    detail.firstElementChild.append(open);
    return;
  }
  document.title = `${project.name}${conversations ? '' : ' · Settings'} · Ahh`;
  if (conversations) {
    terminalDispose = renderConversation({ detail, sidebar, project, refresh: () => navigate(path, true) });
    return;
  }
  renderProject({ detail, project, signal: requests.signal, refresh: () => navigate(path, true), onDeleted: (path, replace) => { focusProjects = true; navigate(path, replace); } });
});
