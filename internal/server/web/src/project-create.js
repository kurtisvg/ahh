import { mutate } from './project-actions.js';

function projectID(name) {
  return name.normalize('NFKD').replace(/\p{M}/gu, '').toLowerCase()
    .replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')
    .slice(0, 64).replace(/-$/, '');
}

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
    <form novalidate>
      <h2>Add Project</h2>
      <div class="form-field">
        <label for="create-project-name">Display name</label>
        <input id="create-project-name" name="name" required autofocus
          aria-describedby="create-project-name-error">
        <p id="create-project-name-error" class="field-error" aria-live="polite"></p>
      </div>
      <div class="form-field">
        <label for="create-project-id">Project ID</label>
        <input id="create-project-id" name="id" required spellcheck="false"
          aria-describedby="project-id-help create-project-id-error">
        <p id="project-id-help" class="field-help">
          Used in the project URL. Use 1–64 lowercase letters or digits,
          separated by single hyphens.
        </p>
        <p id="create-project-id-error" class="field-error" aria-live="polite"></p>
      </div>
      <p role="status"></p>
      <p role="alert"></p>
      <div class="actions">
        <button type="button" class="secondary">Cancel</button>
        <button class="primary" type="submit">Create Project</button>
      </div>
    </form>
  `;
  sidebar.append(dialog);
  const name = dialog.querySelector('[name="name"]');
  const id = dialog.querySelector('[name="id"]');
  const nameError = dialog.querySelector('#create-project-name-error');
  const idError = dialog.querySelector('#create-project-id-error');
  const submit = dialog.querySelector('[type="submit"]');
  const cancel = dialog.querySelector('[type="button"]');
  const status = dialog.querySelector('[role="status"]');
  const error = dialog.querySelector('[role="alert"]');
  let busy = false;
  let customID = false;
  let nameTouched = false;
  let idTouched = false;
  let conflict = '';

  function validate() {
    const displayName = name.value.trim();
    let nameMessage = '';
    if (!displayName) nameMessage = 'Enter a display name.';
    else if (Array.from(displayName).length > 100 || /\p{Cc}/u.test(displayName)) {
      nameMessage = 'Use 1–100 characters without control characters.';
    }
    let idMessage = conflict;
    if (!id.value) idMessage = 'Enter a project ID.';
    else if (id.value.length > 64) idMessage = 'Use no more than 64 characters.';
    else if (!/^[a-z0-9]+(-[a-z0-9]+)*$/.test(id.value)) {
      idMessage = 'Use lowercase letters or digits, separated by single hyphens.';
    }
    nameError.textContent = nameTouched ? nameMessage : '';
    idError.textContent = idTouched ? idMessage : '';
    name.setAttribute('aria-invalid', String(nameTouched && Boolean(nameMessage)));
    id.setAttribute('aria-invalid', String(idTouched && Boolean(idMessage)));
    submit.disabled = busy || Boolean(nameMessage || idMessage);
    return !nameMessage && !idMessage;
  }

  name.oninput = () => {
    nameTouched = true;
    if (!customID) {
      const generated = projectID(name.value);
      if (id.value !== generated) conflict = '';
      id.value = generated;
      idTouched = true;
    }
    validate();
  };
  id.oninput = () => {
    customID = idTouched = true;
    conflict = '';
    validate();
  };
  name.onblur = () => { nameTouched = true; validate(); };
  id.onblur = () => { idTouched = true; validate(); };
  button.onclick = () => {
    error.textContent = '';
    dialog.showModal();
  };
  cancel.onclick = () => dialog.close();
  dialog.oncancel = event => {
    if (busy) event.preventDefault();
  };
  validate();
  dialog.querySelector('form').onsubmit = async event => {
    event.preventDefault();
    if (busy) return;
    nameTouched = idTouched = true;
    if (!validate()) return;
    name.disabled = id.disabled = submit.disabled = cancel.disabled = busy = true;
    status.textContent = 'Creating project…';
    error.textContent = '';
    try {
      const project = await mutate('api/projects', {
        id: id.value,
        name: name.value.trim(),
      }, signal);
      if (!signal.aborted) navigate(`p/${project.id}`);
    } catch (reason) {
      if (!signal.aborted) {
        if (reason.code === 'conflict') conflict = reason.message;
        else error.textContent = `Could not create project. ${reason.message}`;
      }
    } finally {
      name.disabled = id.disabled = cancel.disabled = busy = false;
      status.textContent = '';
      validate();
    }
  };
}
