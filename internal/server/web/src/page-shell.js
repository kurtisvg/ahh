import { link } from './navigation.js';
import { icon } from './icons.js';

export function renderShell(app, { id, conversations }) {
  app.innerHTML = `
    <nav id="activity" aria-label="Pages"></nav>
    <div id="page-header"></div>
    <aside id="sidebar"></aside>
    <section id="detail" aria-live="${conversations ? 'off' : 'polite'}"></section>
  `;
  const activity = document.querySelector('#activity');
  const sidebar = document.querySelector('#sidebar');
  const detail = document.querySelector('#detail');
  app.classList.toggle('conversations-page', conversations);
  const projectsLink = link(id ? `p/${id}` : 'p', '');
  projectsLink.title = 'Settings';
  projectsLink.setAttribute('aria-label', 'Settings');
  projectsLink.append(icon('settings'));
  const pageHeader = document.querySelector('#page-header');
  pageHeader.innerHTML = '<h1>Settings</h1><nav class="settings-tabs" aria-label="Settings sections"></nav>';
  const projectsTab = link(id ? `p/${id}` : 'p', 'Projects');
  projectsTab.setAttribute('aria-current', 'page');
  pageHeader.querySelector('nav').append(projectsTab);
  const conversationsLink = link(id ? `p/${id}/c` : 'c', '');
  conversationsLink.append(icon('conversations'));
  conversationsLink.title = 'Conversations';
  conversationsLink.setAttribute('aria-label', 'Conversations');
  activity.append(conversationsLink, projectsLink);
  return { sidebar, detail, projectsLink, conversationsLink };
}
