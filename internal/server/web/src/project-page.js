import { link } from './navigation.js';
import { editName } from './project-actions.js';
import { projectDeletion } from './project-delete.js';

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
  if (['deleting', 'delete_failed'].includes(project.state)) {
    const notice = document.createElement('p');
    notice.setAttribute('role', 'alert');
    notice.textContent = project.state === 'deleting'
      ? 'Deletion is pending. If it was interrupted, check the server logs; automatic recovery is not available yet.'
      : project.error || 'Deletion did not finish. Retry to remove the remaining files and project record.';
    panel.append(notice);
  }
  projectDeletion(panel, project, signal, onDeleted);
}
