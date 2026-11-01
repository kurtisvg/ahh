import { mutate, forgetProject } from './project-actions.js';

export function projectDeletion(detail, project, signal, navigate) {
  const button = document.createElement('button');
  button.className = 'danger';
  button.type = 'button';
  button.id = 'delete-project';
  button.textContent = project.state === 'delete_failed' ? 'Retry deletion' : 'Delete Project';
  button.disabled = ['creating', 'deleting'].includes(project.state);
  detail.querySelector('.delete-slot').append(button);
  const dialog = document.createElement('dialog');
  dialog.setAttribute('aria-label', 'Delete Project');
  dialog.innerHTML = `
    <h2></h2>
    <p>Delete this project and all its files, including local commits and
      uncommitted work? Its running conversation will be stopped.</p>
    <p role="alert"></p>
    <div class="actions">
      <button class="secondary" autofocus>Cancel</button>
      <button class="danger">Delete Project</button>
    </div>
  `;
  dialog.querySelector('h2').textContent = `Delete ${project.name}?`;
  detail.append(dialog);
  const [cancel, confirm] = dialog.querySelectorAll('button');
  let busy = false;
  cancel.onclick = () => dialog.close();
  dialog.oncancel = event => { if (busy) event.preventDefault(); };
  button.onclick = () => dialog.showModal();
  confirm.onclick = async () => {
    if (busy) return;
    busy = button.disabled = cancel.disabled = confirm.disabled = true;
    confirm.textContent = 'Deleting…';
    dialog.querySelector('[role="alert"]').textContent = '';
    try {
      await mutate(`api/projects/${project.id}?discard=true`, undefined, signal, 'DELETE');
      forgetProject(project.id);
      if (!signal.aborted) navigate('p', true);
    } catch (error) {
      if (!signal.aborted) dialog.querySelector('[role="alert"]').textContent = `Deletion could not finish. ${error.message}`;
    } finally {
      busy = button.disabled = cancel.disabled = confirm.disabled = false;
      confirm.textContent = 'Retry deletion';
    }
  };
}
