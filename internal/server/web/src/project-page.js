export function renderProject({ detail, project }) {
  detail.innerHTML = `
    <h2></h2>
    <dl>
      <dt>Name</dt>
      <dd id="project-name"></dd>
      <dt>Type</dt>
      <dd>Git</dd>
    </dl>
  `;
  detail.querySelector('h2').textContent = `${project.name} settings`;
  detail.querySelector('#project-name').textContent = project.name;
}
