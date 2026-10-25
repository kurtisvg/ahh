import { link } from './navigation.js';
import { editName } from './project-actions.js';

export function renderProject({ detail, project, signal, refresh, onDeleted }) {
  detail.innerHTML = `
    <div class="settings-panel">
      <div class="section-heading">
        <h2>
          <span></span>
          <span class="tag">Git</span>
        </h2>
      </div>
    </div>
  `;
  const panel = detail.firstElementChild;
  panel.querySelector('h2 span').textContent = project.name;
  const open = link(`p/${project.id}/c`, 'Open conversations');
  open.className = 'button secondary';
  panel.querySelector('.section-heading').append(open);
  editName(panel, project, signal, refresh);
}
