import { mutate } from './project-actions.js';

export function addProject(sidebar, signal, navigate) {
  const button = document.createElement('button');
  button.textContent = '+';
  button.className = 'icon-button';
  button.title = 'Add Project';
  button.setAttribute('aria-label', 'Add Project');
  sidebar.querySelector('h2').append(button);
  const dialog = document.createElement('dialog');
  dialog.setAttribute('aria-label', 'Add Project');
  dialog.innerHTML = `
    <form>
      <h2>Add Project</h2>
      <label>Project ID
        <input name="id" required autofocus maxlength="64"
          pattern="[a-z0-9]+(-[a-z0-9]+)*" aria-describedby="project-id-help">
      </label>
      <p id="project-id-help">
        Use lowercase letters, digits, and single hyphens, up to 64 characters.
      </p>
      <p role="status"></p>
      <p role="alert"></p>
      <div class="actions">
        <button type="button" class="secondary">Cancel</button>
        <button class="primary" type="submit">Create Project</button>
      </div>
    </form>
  `;
  sidebar.append(dialog);
  const input = dialog.querySelector('input');
  const submit = dialog.querySelector('[type="submit"]');
  const cancel = dialog.querySelector('[type="button"]');
  const status = dialog.querySelector('[role="status"]');
  const error = dialog.querySelector('[role="alert"]');
  let busy = false;
  button.onclick = () => {
    error.textContent = '';
    dialog.showModal();
  };
  cancel.onclick = () => dialog.close();
  dialog.oncancel = event => {
    if (busy) event.preventDefault();
  };
  dialog.querySelector('form').onsubmit = async event => {
    event.preventDefault();
    if (busy) return;
    input.disabled = submit.disabled = cancel.disabled = busy = true;
    status.textContent = 'Creating project…';
    error.textContent = '';
    try {
      const project = await mutate('api/projects', { id: input.value }, signal);
      if (!signal.aborted) navigate(`p/${project.id}`);
    } catch (reason) {
      if (!signal.aborted) error.textContent = `Could not create project. ${reason.message}`;
    } finally {
      input.disabled = submit.disabled = cancel.disabled = busy = false;
      status.textContent = '';
    }
  };
}
