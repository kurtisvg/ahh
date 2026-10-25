import { appURL } from './router.js';

export async function mutate(path, body, signal, method = 'POST') {
  const response = await fetch(appURL(path), {
    method, signal, headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (response.status === 204) return null;
  const result = await response.json();
  if (!response.ok) {
    const error = new Error(result.message || 'Project operation failed');
    error.current = result.current;
    throw error;
  }
  return result;
}

// Drafts survive page navigation within this tab; their version stays tied to
// the original read so another tab's changes cannot be silently overwritten.
const drafts = new Map();
const saved = new Set();
export function editName(detail, project, signal, refresh) {
  const form = document.createElement('form');
  form.id = 'project-form';
  form.innerHTML = `
    <label>Project name <input id="project-name" required></label>
    <p role="status"></p>
    <p role="alert"></p>
    <div class="form-actions">
      <div class="delete-slot"></div>
      <div class="save-actions">
        <button type="button" class="secondary">Discard changes</button>
        <button type="submit" class="primary">Save changes</button>
      </div>
    </div>
  `;
  detail.append(form);
  const feedback = form.querySelector('[role=alert]');
  const status = form.querySelector('[role=status]');
  if (saved.has(project.id)) status.textContent = 'Saved';
  const input = form.querySelector('input');
  const draft = drafts.get(project.id) || { name: project.name, version: project.version };
  input.value = draft.name;
  input.oninput = () => {
    draft.name = input.value;
    drafts.set(project.id, draft);
    saved.delete(project.id);
    feedback.textContent = '';
    status.textContent = '';
    updateControls();
  };
  const discard = form.querySelector('[type="button"]');
  discard.onclick = () => {
    drafts.delete(project.id);
    saved.delete(project.id);
    refresh();
  };
  const submit = form.querySelector('[type="submit"]');
  let saving = false;
  function updateControls() {
    const changed = input.value !== project.name || draft.version !== project.version;
    input.disabled = saving || ['creating', 'deleting'].includes(project.state);
    submit.disabled = discard.disabled = input.disabled || !changed;
  }
  updateControls();
  form.onsubmit = async event => {
    event.preventDefault();
    if (submit.disabled) return;
    saving = true;
    updateControls();
    submit.textContent = 'Saving…';
    try {
      await mutate(`api/projects/${project.id}`, draft, signal, 'PATCH');
      drafts.delete(project.id);
      saved.add(project.id);
      if (!signal.aborted) refresh();
    } catch (error) {
      if (!signal.aborted) {
        feedback.textContent = error.message
          + (error.current ? ` Current saved name: ${error.current.name}` : '');
      }
    } finally {
      saving = false;
      submit.textContent = 'Save changes';
      updateControls();
    }
  };
}
