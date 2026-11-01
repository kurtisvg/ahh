// A view owns all terminal callbacks and connections until disposed.
export function mountTerminal({ container, status, start, notice, baseURL = document.baseURI, endpoint, onMissing = () => {}, onEscape = () => {}, onState = () => {} }) {
  let disposed = false;
  let pollTimer;
  let retryTimer;
  const requests = new AbortController();
  function setStatus(text, state = 'neutral') {
    status.textContent = text;
    status.dataset.state = state;
  }
  async function startTerminal() {
    start.disabled = true;
    start.textContent = 'Starting…';
    setStatus('Starting…', 'waiting');
    try {
      const response = await fetch(new URL(endpoint || 'start', baseURL), { method: 'POST', signal: requests.signal });
      if (!response.ok) throw new Error(await response.text());
      if (!disposed) await refreshStatus();
    } catch (error) {
      if (disposed) return;
      notice.textContent = `Could not start terminal: ${error.message}`;
      if (!disposed) await refreshStatus();
    }
  }
  start.addEventListener('click', startTerminal);
  const terminal = new Terminal({
    cursorBlink: true,
    screenReaderMode: true,
    scrollback: 2000,
    disableStdin: true,
    theme: { background: '#11151c', foreground: '#e2e8f0' },
  });
  terminal.attachCustomKeyEventHandler(event => {
    if (event.key === 'Escape' && event.shiftKey) {
      if (event.type === 'keydown') onEscape();
      return false;
    }
    return true;
  });
  const fit = new FitAddon.FitAddon();
  terminal.loadAddon(fit);
  terminal.open(container);
  fit.fit();

  let invocation;
  let refreshing = false;
  let rendered = null;
  let pending = Promise.resolve();
  let exited = false;
  let outputComplete = false;
  let socket;

  function send(message, connection = socket) {
    if (!disposed && connection?.readyState === WebSocket.OPEN) connection.send(JSON.stringify(message));
  }

  function sendInput(data) {
    // JSON escaping needs at most six bytes per UTF-16 code unit. This chunk
    // size leaves room for message fields within the wrapper's 64 KiB limit.
    let chunk = '';
    for (const character of data) {
      // Iterate by code point so a chunk boundary never splits a surrogate pair.
      if (chunk.length + character.length > 8 * 1024) {
        send({ type: 'input', data: chunk });
        chunk = '';
      }
      chunk += character;
    }
    if (chunk) send({ type: 'input', data: chunk });
  }

  terminal.onData(sendInput);
  terminal.onResize(({ rows, cols }) => send({ type: 'resize', rows, cols }));
  const observer = new ResizeObserver(() => { if (!disposed) fit.fit(); });
  observer.observe(container);

  function connect() {
    if (disposed || exited || outputComplete) return;

    // Keep the page's path prefix when connecting through a proxy.
    const url = new URL(endpoint ? `${endpoint}/${invocation}/pty` : 'pty', baseURL);
    url.protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    if (rendered !== null) url.searchParams.set('cursor', rendered.toString());
    const connection = new WebSocket(url);
    socket = connection;
    connection.binaryType = 'arraybuffer';
    let received = null;

    connection.onopen = () => {
      if (disposed || socket !== connection || exited || outputComplete) return;
      setStatus('Connected', 'connected');
      terminal.options.disableStdin = false;
      send({ type: 'resize', rows: terminal.rows, cols: terminal.cols }, connection);
    };

    connection.onmessage = ({ data }) => {
      if (disposed || socket !== connection) return;
      if (data instanceof ArrayBuffer) {
        if (data.byteLength < 8) {
          connection.close();
          return;
        }
        const offset = new DataView(data).getBigUint64(0);
        if (offset !== received) {
          connection.close();
          return;
        }

        const bytes = new Uint8Array(data, 8);
        received = offset + BigInt(bytes.byteLength);
        const end = received;
        // Reconnect from rendered output, including writes queued before disconnect.
        pending = new Promise(resolve => terminal.write(bytes, () => {
          if (disposed || socket !== connection) { resolve(); return; }
          rendered = end;
          send({ type: 'ack', offset: end.toString() }, connection);
          resolve();
        }));
        return;
      }

      const message = JSON.parse(data);
      if (message.type === 'replay') {
        received = BigInt(message.offset);
        rendered = received;
        if (message.gap) {
          terminal.reset();
          notice.textContent = 'Earlier output is no longer available';
        }
      } else if (message.type === 'end' && BigInt(message.offset) === received) {
        outputComplete = true;
        terminal.options.disableStdin = true;
      }
    };

    connection.onclose = async () => {
      if (disposed || socket !== connection) return;
      terminal.options.disableStdin = true;
      if (!exited) setStatus(outputComplete ? 'Disconnected' : 'Reconnecting', outputComplete ? 'disconnected' : 'waiting');
      // xterm processes writes in order; the last callback covers all queued output.
      await pending;
      if (!disposed && !exited && !outputComplete) retryTimer = setTimeout(connect, 500);
    };
  }

  async function refreshStatus() {
    if (disposed || refreshing) return;
    refreshing = true;
    clearTimeout(pollTimer);
    try {
      const response = await fetch(new URL(endpoint || 'status', baseURL), { cache: 'no-store', signal: AbortSignal.any([requests.signal, AbortSignal.timeout(5000)]) });
      if (disposed) return;
      if (response.status === 404 && endpoint) { onMissing(); return; }
      if (!response.ok) throw new Error('Status unavailable');

      const process = await response.json();
      if (disposed) return;
      if (process.invocation !== invocation) {
        const old = socket; socket = null; old?.close();
        clearTimeout(retryTimer);
        if (invocation) terminal.reset();
        invocation = process.invocation;
        rendered = null; exited = false; outputComplete = false;
      }
      start.hidden = process.state === 'running';
      start.disabled = process.start_allowed === false || (process.state !== 'stopped' && process.state !== 'exited');
      start.textContent = process.state === 'starting' ? 'Starting…' : 'Start conversation';
      onState(process);
      if (process.error) notice.textContent = process.error;
      const unavailable = 'Project storage is unavailable or an operation is in progress.';
      if (process.start_allowed === false) notice.textContent = unavailable;
      else if (notice.textContent === unavailable) notice.textContent = '';
      if (process.state === 'starting') setStatus('Starting…', 'waiting');
      if (process.state === 'stopping') setStatus('Stopping…', 'waiting');
      if (process.state === 'disconnected') { setStatus('Disconnected', 'disconnected'); terminal.options.disableStdin = true; }
      if (process.state === 'running' && !socket) {
        connect();
      } else if (process.state === 'stopped' || process.state === 'exited') {
        exited = !!process.invocation || process.state === 'exited';
        const reason = process.signal || (process.exit_code == null ? '' : `exit ${process.exit_code}`);
        setStatus(reason ? `Stopped (${reason})` : 'Stopped');
        terminal.options.disableStdin = true;
      } else if (process.state === 'failed') {
        exited = true;
        setStatus('Wrapper exited without a result', 'disconnected');
        terminal.options.disableStdin = true;
      }
    } catch {
      if (!disposed) { start.disabled = true; if (!socket || socket.readyState !== WebSocket.OPEN) setStatus('Disconnected', 'disconnected'); }
    } finally {
      // Wait for each request to finish before scheduling the next poll.
      refreshing = false;
      if (!disposed) pollTimer = setTimeout(refreshStatus, 1000);
    }
  }

  refreshStatus();

  return () => {
    if (disposed) return;
    disposed = true;
    requests.abort();
    clearTimeout(pollTimer);
    clearTimeout(retryTimer);
    start.removeEventListener('click', startTerminal);
    observer.disconnect();
    socket?.close();
    terminal.dispose();
  };
}
