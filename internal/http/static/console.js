// SSH consoles (xterm over WebSocket) for the server and app detail pages. xterm itself is loaded on demand (app.js loadXterm).
function dockifyConsole(cfg) {
  var term = null, ws = null, onResize = null;
  function el(id) { return document.getElementById(id); }
  function setStatus(text, cls) {
    var s = el(cfg.status);
    s.textContent = text;
    s.className = 'badge ' + cls;
  }
  function connect() {
    var id = el(cfg.toggle).getAttribute(cfg.idAttr);
    if (!id) return;
    var base = document.querySelector('base').getAttribute('href') || '/';
    var proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    var enc = new TextEncoder();
    term = new Terminal({ fontFamily: cfg.fontFamily, fontSize: 13, lineHeight: 1.35, cursorBlink: true, cursorStyle: 'block', theme: cfg.theme });
    var fit = new FitAddon.FitAddon();
    term.loadAddon(fit);
    term.open(el(cfg.terminal));
    setTimeout(function () { fit.fit(); }, 50);
    ws = new WebSocket(proto + '//' + window.location.host + base + 'api/' + cfg.kind + '/' + id + '/console');
    ws.binaryType = 'arraybuffer';
    function sendSize() {
      var d = fit.proposeDimensions();
      if (d && ws && ws.readyState === WebSocket.OPEN) ws.send(enc.encode(JSON.stringify({ resize: { width: d.cols, height: d.rows } })));
    }
    ws.onopen = function () { setStatus('Connected', 'badge-online'); term.focus(); sendSize(); };
    ws.onmessage = function (evt) { term.write(evt.data instanceof ArrayBuffer ? new Uint8Array(evt.data) : evt.data); };
    ws.onclose = function () {
      setStatus('Disconnected', 'badge-offline');
      if (term) term.write('\r\n\x1b[31mSession ended.\x1b[0m\r\n');
    };
    term.onData(function (data) { if (ws && ws.readyState === WebSocket.OPEN) ws.send(data); });
    onResize = function () { if (ws && ws.readyState === WebSocket.OPEN) { fit.fit(); sendSize(); } };
    window.addEventListener('resize', onResize);
  }
  function disconnect() {
    if (onResize) { window.removeEventListener('resize', onResize); onResize = null; }
    if (ws) { ws.close(); ws = null; }
    if (term) { term.dispose(); term = null; }
    setStatus('Disconnected', 'badge-offline');
  }
  var api = {
    disconnect: disconnect,
    toggle: function () {
      var box = el(cfg.container), btn = el(cfg.toggle);
      if (box.hidden) {
        box.hidden = false;
        if (cfg.openLabel) btn.textContent = cfg.openLabel;
        loadXterm(connect);
      } else {
        box.hidden = true;
        if (cfg.closedLabel) btn.textContent = cfg.closedLabel;
        disconnect();
      }
    },
  };
  return api;
}
var serverConsole = null, appConsole = null;
function toggleConsole() {
  serverConsole = serverConsole || dockifyConsole({
    kind: 'servers', idAttr: 'data-server-id', container: 'console-container', toggle: 'console-toggle', terminal: 'terminal', status: 'console-status',
    openLabel: 'Close Console', closedLabel: 'Console', fontFamily: '"Fira Code", "JetBrains Mono", ui-monospace, monospace', theme: {
      	background: '#090d16',
      	foreground: '#f8fafc',
      	cursor: '#6366f1',
      	selectionBackground: '#334155',
      	black: '#090d16',
      	red: '#f87171',
      	green: '#34d399',
      	yellow: '#fbbf24',
      	blue: '#60a5fa',
      	magenta: '#c084fc',
      	cyan: '#38bdf8',
      	white: '#f8fafc',
      	brightBlack: '#64748b',
      	brightRed: '#f87171',
      	brightGreen: '#34d399',
      	brightYellow: '#fbbf24',
      	brightBlue: '#60a5fa',
      	brightMagenta: '#c084fc',
      	brightCyan: '#38bdf8',
      	brightWhite: '#ffffff'
      } });
  serverConsole.toggle();
}
function toggleAppConsole() {
  appConsole = appConsole || dockifyConsole({
    kind: 'apps', idAttr: 'data-app-id', container: 'app-console-container', toggle: 'app-console-toggle', terminal: 'app-terminal', status: 'app-console-status',
    fontFamily: '"Berkeley Mono", "IBM Plex Mono", ui-monospace, monospace', theme: {
      	background: '#0d0d0d',
      	foreground: '#cfcecd',
      	cursor: '#cfcecd',
      	selectionBackground: '#3a3a3a',
      	black: '#0d0d0d',
      	red: '#b33a3a',
      	green: '#3d8b4a',
      	yellow: '#b8942e',
      	blue: '#4a7bbb',
      	magenta: '#b84a7b',
      	cyan: '#4ab8b8',
      	white: '#cfcecd',
      	brightBlack: '#555',
      	brightRed: '#b33a3a',
      	brightGreen: '#3d8b4a',
      	brightYellow: '#b8942e',
      	brightBlue: '#4a7bbb',
      	brightMagenta: '#b84a7b',
      	brightCyan: '#4ab8b8',
      	brightWhite: '#cfcecd'
      } });
  appConsole.toggle();
}
// A boosted navigation replaces #main; close any open console so its socket does not leak.
(window.dockifyCleanups = window.dockifyCleanups || []).push(function () {
  if (serverConsole) { serverConsole.disconnect(); serverConsole = null; }
  if (appConsole) { appConsole.disconnect(); appConsole = null; }
});
