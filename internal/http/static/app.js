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

// Inline page scripts that need a deferred asset (charts.js, console.js): on a full load they run before it, so wait for DCL.
function dockifyReady(fn) {
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', fn, { once: true });
  else fn();
}

// xterm is only needed for the SSH console: load it on first use.
var xtermLoading = null;
function loadXterm(cb) {
  if (window.Terminal && window.FitAddon) return cb();
  if (!xtermLoading) {
    xtermLoading = new Promise(function (resolve, reject) {
      var l = document.createElement('link');
      l.rel = 'stylesheet';
      l.href = 'static/xterm.min.css?v=' + assetV;
      l.setAttribute('hx-preserve', 'true');
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

// ---- S9: boosted navigation (hx-boost on <body>; only #main is swapped, header/tab bar persist) ----
// Page scripts must be idempotent: run on DOMContentLoaded and again on htmx:afterSettle, guarded by data-init.
function formatTimes(root) {
  [].forEach.call((root || document).querySelectorAll('time[datetime]:not([data-init])'), function (el) {
    el.setAttribute('data-init', '1');
    var d = new Date(el.getAttribute('datetime'));
    if (isNaN(d)) return;
    var p = function (n) { return String(n).padStart(2, '0'); };
    el.textContent = d.getFullYear() + '-' + p(d.getMonth() + 1) + '-' + p(d.getDate()) + ' ' + p(d.getHours()) + ':' + p(d.getMinutes()) + ':' + p(d.getSeconds());
  });
}
function syncNav() {
  var m = document.getElementById('main'), cur = m && m.getAttribute('data-nav');
  [].forEach.call(document.querySelectorAll('.main-nav a[data-nav]'), function (a) {
    if (a.getAttribute('data-nav') === cur) a.setAttribute('aria-current', 'page');
    else a.removeAttribute('aria-current');
  });
}
// Auto-refresh of status pages (deploying/initializing): <main data-poll="2000"> re-fetches itself; the timer is a page cleanup.
var pollTimer = null;
function startPoll() {
  clearTimeout(pollTimer);
  var m = document.getElementById('main'), ms = m && Number(m.getAttribute('data-poll'));
  if (!ms || !window.htmx) return;
  pollTimer = setTimeout(function () {
    htmx.ajax('GET', location.href, { target: '#main', select: '#main', swap: 'outerHTML' });
  }, ms);
  (window.dockifyCleanups = window.dockifyCleanups || []).push(function () { clearTimeout(pollTimer); });
}
// Everything a page needs after its #main arrives (swap or history restore); idempotent.
function pageInit() {
  formatTimes();
  syncNav();
  if (window.dockifyApplyTheme) window.dockifyApplyTheme(); // head merge replaces <meta name=theme-color>
  startPoll();
}
document.addEventListener('DOMContentLoaded', function () { formatTimes(); startPoll(); });
if (window.htmx) {
  var progressTimer = null;
  function progress(show) {
    clearTimeout(progressTimer);
    var bar = document.getElementById('progress');
    if (!bar) return;
    if (!show) { bar.hidden = true; return; }
    progressTimer = setTimeout(function () { bar.hidden = false; }, 150);
  }
  document.addEventListener('htmx:beforeRequest', function (e) {
    var d = e.detail;
    // Preload requests are intercepted by the preload extension and must not flash the bar.
    if (d.boosted && !(d.requestConfig.headers || {})['HX-Preloaded']) progress(true);
  });
  ['htmx:afterSettle', 'htmx:responseError', 'htmx:sendError', 'htmx:timeout', 'htmx:swapError'].forEach(function (n) {
    document.addEventListener(n, function () { progress(false); });
  });
  document.addEventListener('htmx:beforeSwap', function (e) {
    var d = e.detail;
    if (d.boosted) {
      // Login redirect, session expiry, anything without #main: fall back to a full page load.
      var doc = new DOMParser().parseFromString(d.xhr.responseText, 'text/html');
      if (!doc.getElementById('main')) {
        d.shouldSwap = false;
        progress(false);
        // A GET can be repeated as a normal navigation; a POST must not be replayed as one.
        if (d.requestConfig.verb === 'get') location.assign(d.xhr.responseURL || location.href);
        else showToast('The server did not return a page. Reload and check the result.', true);
        return;
      }
      // 4xx/5xx pages (error.html) are shown like any other page.
      if (d.xhr.status >= 400) { d.shouldSwap = true; d.isError = false; }
    }
    if (d.target && d.target.id === 'main') {
      runCleanups();
    }
  });
  // head-support: never drop scripts/styles already loaded (fonts link mutates media=all, xterm is added at runtime).
  document.addEventListener('htmx:removingHeadElement', function (e) {
    var t = e.detail.headElement.tagName;
    if (t === 'LINK' || t === 'SCRIPT') e.preventDefault();
  });
  document.addEventListener('htmx:addingHeadElement', function (e) {
    var el = e.detail.headElement.firstElementChild;
    if (!el) return;
    var url = el.getAttribute('href') || el.getAttribute('src');
    if (url && document.head.querySelector((el.tagName === 'LINK' ? 'link[href="' : 'script[src="') + url + '"]')) e.preventDefault();
  });
  document.addEventListener('htmx:afterSettle', function (e) {
    pageInit();
    if (e.detail.boosted) window.scrollTo(0, 0);
  });
  // Back/Forward never fire beforeSwap/afterSettle: close live sockets before the restored body is swapped in, re-init after.
  function runCleanups() {
    var c = window.dockifyCleanups || [];
    window.dockifyCleanups = [];
    c.forEach(function (f) { try { f(); } catch (err) {} });
  }
  document.addEventListener('htmx:historyCacheMissLoad', runCleanups);
  document.addEventListener('htmx:historyRestore', pageInit);
  // Back/forward: do not restore stale DOM snapshots; fetch the page again.
  htmx.config.historyCacheSize = 0;
}
// Light PWA: the service worker only caches /static/*.
if ('serviceWorker' in navigator) {
  window.addEventListener('load', function () {
    navigator.serviceWorker.register('static/sw.js?v=' + assetV, { scope: './' }).catch(function () {});
  });
}

// Log output is plain text: show empty results and HTTP errors instead of a silent black box.
document.addEventListener('htmx:afterSwap', function (e) {
  var t = e.detail.target;
  if (t && t.classList.contains('log-content') && !t.textContent.trim()) t.textContent = '(no output)';
});
document.addEventListener('htmx:responseError', function (e) {
  var t = e.detail.target;
  if (t && t.classList.contains('log-content')) t.textContent = 'Error ' + e.detail.xhr.status + ': ' + (e.detail.xhr.responseText || '').slice(0, 2000);
});
