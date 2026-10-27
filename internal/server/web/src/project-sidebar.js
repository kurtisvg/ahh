import { link } from './navigation.js';

export function projectList(projects, selectedID) {
  const list = document.createElement('nav');
  list.setAttribute('aria-label', 'Projects');
  for (const project of projects) {
    const duplicate = projects.some(other => other.id !== project.id && other.name === project.name);
    const label = `${project.name}${duplicate ? ` (${project.id.slice(0, 8)})` : ''} · ${project.state}`;
    const a = link(`p/${project.id}`, label);
    if (project.id === selectedID) a.setAttribute('aria-current', 'page');
    list.append(a);
  }
  return list;
}
