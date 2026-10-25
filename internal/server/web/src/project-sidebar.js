import { link } from './navigation.js';

export function projectList(projects, selectedID) {
  const list = document.createElement('nav');
  list.setAttribute('aria-label', 'Projects');
  for (const item of projects) {
    const duplicate = projects.some(other => other.id !== item.id && other.name === item.name);
    const label = `${item.name}${duplicate ? ` (${item.id})` : ''}${item.state === 'ready' ? '' : ` · ${item.state}`}`;
    const a = link(`p/${item.id}`, label);
    if (item.id === selectedID) a.setAttribute('aria-current', 'page');
    list.append(a);
  }
  return list;
}

export function renderProjectPicker(sidebar, projects, selectedID, navigate) {
  sidebar.innerHTML = `
    <div class="project-picker">
      <select id="project-select" aria-label="Project"></select>
    </div>
    <h2 class="conversation-list-heading">Conversations</h2>
  `;
  const select = sidebar.querySelector('select');
  for (const p of projects) {
    const duplicate = projects.some(other => other.id !== p.id && other.name === p.name);
    const option = new Option(p.name + (duplicate ? ` (${p.id})` : ''), p.id);
    select.add(option);
  }
  select.value = selectedID || '';
  select.disabled = projects.length === 0;
  if (!projects.length) select.add(new Option('No Projects', ''));
  select.onchange = () => navigate(`p/${select.value}/c`);
}
