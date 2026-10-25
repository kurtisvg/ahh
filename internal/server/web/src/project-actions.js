import { appURL } from './router.js';

export async function mutate(path, body, signal, method = 'POST') {
  const response = await fetch(appURL(path), {
    method, signal, headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const result = await response.json();
  if (!response.ok) throw new Error(result.message || 'Project operation failed');
  return result;
}

