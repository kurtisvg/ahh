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

let received = 0n;
let exited = false;

// Keep the page's path prefix when connecting through a proxy.
const url = new URL('pty', location.href);
url.protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
const socket = new WebSocket(url);
socket.binaryType = 'arraybuffer';

function send(message) {
  if (socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify(message));
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

socket.onopen = () => {
  if (exited) return;
  status.textContent = 'Connected';
  terminal.options.disableStdin = false;
  send({ type: 'resize', rows: terminal.rows, cols: terminal.cols });
  terminal.focus();
};

socket.onmessage = ({ data }) => {
  if (data instanceof ArrayBuffer) {
    const offset = new DataView(data).getBigUint64(0);
    if (offset !== received) {
      socket.close();
      return;
    }

    const bytes = new Uint8Array(data, 8);
    received = offset + BigInt(bytes.byteLength);
    const end = received;
    // Acknowledge rendering, rather than just receipt, to bound pending output.
    terminal.write(bytes, () => send({ type: 'ack', offset: end.toString() }));
    return;
  }

  const message = JSON.parse(data);
  if (message.type === 'replay') {
    received = BigInt(message.offset);
    if (message.gap) {
      terminal.reset();
      document.querySelector('#notice').textContent = 'Earlier output is no longer available';
    }
  }
};

socket.onclose = () => {
  terminal.options.disableStdin = true;
  if (!exited) status.textContent = 'Disconnected';
};

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

refreshStatus();
