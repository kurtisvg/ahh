const status = document.querySelector('#status');
const terminal = new Terminal({
  cursorBlink: true,
  screenReaderMode: true,
  scrollback: 2000,
  disableStdin: true,
  theme: { background: '#11151c', foreground: '#e2e8f0' },
});
const fit = new FitAddon.FitAddon();
terminal.loadAddon(fit);
terminal.open(document.querySelector('#terminal'));
fit.fit();

let rendered = null;
let pending = Promise.resolve();
let exited = false;
let outputComplete = false;
let socket;

function send(message, connection = socket) {
  if (connection?.readyState === WebSocket.OPEN) connection.send(JSON.stringify(message));
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
new ResizeObserver(() => fit.fit()).observe(document.querySelector('#terminal'));

function connect() {
  if (exited || outputComplete) return;

  // Keep the page's path prefix when connecting through a proxy.
  const url = new URL('pty', location.href);
  url.protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
  if (rendered !== null) url.searchParams.set('cursor', rendered.toString());
  const connection = new WebSocket(url);
  socket = connection;
  connection.binaryType = 'arraybuffer';
  let received = null;

  connection.onopen = () => {
    if (exited || outputComplete) return;
    status.textContent = 'Connected';
    terminal.options.disableStdin = false;
    send({ type: 'resize', rows: terminal.rows, cols: terminal.cols }, connection);
    terminal.focus();
  };

  connection.onmessage = ({ data }) => {
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
        document.querySelector('#notice').textContent = 'Earlier output is no longer available';
      }
    } else if (message.type === 'end' && BigInt(message.offset) === received) {
      outputComplete = true;
      terminal.options.disableStdin = true;
    }
  };

  connection.onclose = async () => {
    terminal.options.disableStdin = true;
    if (!exited) status.textContent = outputComplete ? 'Disconnected' : 'Disconnected; reconnecting';
    // xterm processes writes in order; the last callback covers all queued output.
    await pending;
    if (!exited && !outputComplete) setTimeout(connect, 500);
  };
}

async function refreshStatus() {
  try {
    const response = await fetch('status', { cache: 'no-store', signal: AbortSignal.timeout(5000) });
    if (!response.ok) return;

    const process = await response.json();
    if (process.state === 'exited') {
      exited = true;
      const reason = process.signal || (process.exit_code == null ? '' : `exit ${process.exit_code}`);
      status.textContent = reason ? `Stopped (${reason})` : 'Stopped';
      terminal.options.disableStdin = true;
    } else if (process.state === 'failed') {
      exited = true;
      status.textContent = 'Wrapper exited without a result';
      terminal.options.disableStdin = true;
    }
  } catch {
    // Loss of contact does not confirm process exit.
  } finally {
    // Wait for each request to finish before scheduling the next poll.
    if (!exited) setTimeout(refreshStatus, 1000);
  }
}

connect();
refreshStatus();
