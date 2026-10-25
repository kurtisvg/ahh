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
  editName(panel, project, signal, refresh);
}
