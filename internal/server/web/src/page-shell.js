import { link } from './navigation.js';

export function renderShell(app) {
  app.innerHTML = `
    <nav id="activity" aria-label="Pages"></nav>
    <aside id="sidebar"></aside>
    <section id="detail" tabindex="-1"></section>
  `;
  const activity = app.querySelector('#activity');
  const sidebar = app.querySelector('#sidebar');
  const detail = app.querySelector('#detail');
  const projectsLink = link('p', '▣');
  projectsLink.title = 'Projects';
  projectsLink.setAttribute('aria-label', 'Projects');
  activity.append(projectsLink);
  return { sidebar, detail, projectsLink };
}
