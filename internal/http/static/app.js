var assetV = (document.currentScript && document.currentScript.src.split("?v=")[1]) || "";
// Toast for client-side messages (role=status, or role=alert for errors).
function showToast(msg, isError) {
  var t = document.createElement("div");
  t.className = "toast";
  t.setAttribute("role", isError ? "alert" : "status");
  t.textContent = msg;
  document.body.appendChild(t);
  setTimeout(function () { t.remove(); }, 4000);
}
// Sheets (<dialog class="sheet">): openSheet/closeSheet, backdrop click closes, Escape is native.
function openSheet(id) {
  var d = document.getElementById(id);
  if (!d) return;
  d.showModal();
  var f = d.querySelector('textarea, input:not([type=hidden]), .sheet-foot .btn');
  if (f) f.focus();
}
function closeSheet(id) {
  var d = document.getElementById(id);
  if (d && d.open) d.close();
}
// Close only when pointerdown and click both land on the backdrop, outside the dialog box
// (a click on the handle or a text selection released outside must not close it).
var sheetDown = null;
function outsideSheet(d, e) {
  var r = d.getBoundingClientRect();
  return e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom;
}
document.addEventListener('pointerdown', function (e) {
  var d = e.target;
  sheetDown = d.classList && d.classList.contains('sheet') && outsideSheet(d, e) ? d : null;
});
document.addEventListener('click', function (e) {
  var d = e.target;
  if (d.classList && d.classList.contains('sheet') && d === sheetDown && outsideSheet(d, e)) d.close();
  sheetDown = null;
});

// Segmented controls: keep aria-pressed in sync (the header sits outside the htmx-swapped body).
document.addEventListener('click', function (e) {
  var b = e.target.closest && e.target.closest('.seg-btn');
  if (!b) return;
  [].forEach.call(b.parentNode.children, function (s) { s.setAttribute('aria-pressed', String(s === b)); });
});
// Copy button: data-copy="#selector" copies that element's text (execCommand fallback for plain http).
document.addEventListener('click', function (e) {
  var b = e.target.closest && e.target.closest('[data-copy]');
  if (!b) return;
  var src = document.querySelector(b.getAttribute('data-copy'));
  if (!src) return;
  var isField = src.tagName === 'INPUT' || src.tagName === 'TEXTAREA';
  var text = (isField ? src.value : src.textContent).trim();
  function done(ok) {
    var old = b.innerHTML;
    b.textContent = ok ? 'Copied' : 'Copy failed';
    setTimeout(function () { b.innerHTML = old; }, 1500);
  }
  function fallback() {
    if (isField) src.select();
    else {
      var r = document.createRange();
      r.selectNodeContents(src);
      var sel = window.getSelection();
      sel.removeAllRanges();
      sel.addRange(r);
    }
    var ok = false;
    try { ok = document.execCommand('copy'); } catch (err) {}
    done(ok);
  }
  if (navigator.clipboard && window.isSecureContext) navigator.clipboard.writeText(text).then(function () { done(true); }, fallback);
  else fallback();
});

// xterm is only needed for the SSH console: load it on first use.
var xtermLoading = null;
function loadXterm(cb) {
  if (window.Terminal && window.FitAddon) return cb();
  if (!xtermLoading) {
    xtermLoading = new Promise(function (resolve, reject) {
      var l = document.createElement('link');
      l.rel = 'stylesheet';
      l.href = 'static/xterm.min.css?v=' + assetV;
      document.head.appendChild(l);
      function add(src, next) {
        var s = document.createElement('script');
        s.src = src;
        s.onload = next;
        s.onerror = reject;
        document.head.appendChild(s);
      }
      add('static/xterm.min.js?v=' + assetV, function () { add('static/xterm-addon-fit.min.js?v=' + assetV, resolve); });
    });
  }
  xtermLoading.then(cb, function () { xtermLoading = null; });
}
