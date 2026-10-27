// The server sets the document base from its public URL prefix.
export const baseURL = new URL(document.baseURI);
export const appURL = path => new URL(path.replace(/^\//, ''), baseURL);

export function currentPath() {
  return location.pathname.slice(baseURL.pathname.length).replace(/\/$/, '');
}

export function startRouter(render) {
  function navigate(path, replace = false) {
    history[replace ? 'replaceState' : 'pushState'](null, '', appURL(path));
    render(currentPath());
  }
  function clicked(event) {
    const link = event.target.closest('a[data-nav]');
    if (!link || event.defaultPrevented || event.button !== 0 || event.metaKey ||
        event.ctrlKey || event.shiftKey || event.altKey || link.target || link.hasAttribute('download')) return;
    const url = new URL(link.href);
    if (url.origin !== location.origin || !url.pathname.startsWith(baseURL.pathname)) return;
    event.preventDefault();
    navigate(url.pathname.slice(baseURL.pathname.length));
  }
  window.addEventListener('popstate', () => render(currentPath()));
  document.addEventListener('click', clicked);
  queueMicrotask(() => render(currentPath()));
  return navigate;
}
