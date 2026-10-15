// Filter (search + status chips) and sort for [data-list] containers holding <table class="rows">.
(function () {
  function initList(root) {
    var rows = [].slice.call(root.querySelectorAll('tr[data-name]'));
    var search = root.querySelector('[data-list-search]');
    var chips = [].slice.call(root.querySelectorAll('[data-chip]'));
    var count = root.querySelector('[data-list-count]');
    var reset = root.querySelector('[data-list-reset]');
    var empty = root.querySelector('[data-list-empty]');
    var q = '', status = '';

    function apply() {
      var shown = 0;
      rows.forEach(function (r) {
        var hit = (!q || r.getAttribute('data-search').indexOf(q) > -1) && (!status || r.getAttribute('data-status') === status);
        r.hidden = !hit;
        if (hit) shown++;
      });
      [].forEach.call(root.querySelectorAll('[data-group]'), function (g) {
        g.hidden = !g.querySelector('tr[data-name]:not([hidden])');
      });
      count.textContent = shown + ' of ' + rows.length;
      reset.hidden = !(q || status);
      if (empty) empty.hidden = shown > 0;
      chips.forEach(function (c) { c.setAttribute('aria-pressed', String(c.getAttribute('data-chip') === status)); });
    }

    if (search) search.addEventListener('input', function () { q = search.value.trim().toLowerCase(); apply(); });
    chips.forEach(function (c) { c.addEventListener('click', function () { status = c.getAttribute('data-chip'); apply(); }); });
    reset.addEventListener('click', function () { q = ''; status = ''; if (search) search.value = ''; apply(); });

    [].forEach.call(root.querySelectorAll('table.rows'), function (table) {
      var buttons = [].slice.call(table.querySelectorAll('.th-sort'));
      var key = null, dir = 'asc';
      buttons.forEach(function (btn) {
        btn.addEventListener('click', function () {
          var k = btn.getAttribute('data-sort'), num = btn.getAttribute('data-type') === 'number';
          dir = key === k ? (dir === 'asc' ? 'desc' : 'asc') : (num ? 'desc' : 'asc');
          key = k;
          buttons.forEach(function (b) { b.parentNode.setAttribute('aria-sort', 'none'); });
          btn.parentNode.setAttribute('aria-sort', dir === 'asc' ? 'ascending' : 'descending');
          var attr = 'data-' + k, body = table.tBodies[0];
          [].slice.call(body.rows).sort(function (a, b) {
            var x = a.getAttribute(attr) || '', y = b.getAttribute(attr) || '';
            var r = num ? (parseFloat(x) || 0) - (parseFloat(y) || 0) : x.localeCompare(y);
            return dir === 'asc' ? r : -r;
          }).forEach(function (r) { body.appendChild(r); });
        });
      });
    });
    apply();
  }
  // Idempotent: runs on load and after every htmx swap; data-init guards against double wiring.
  function init() {
    [].forEach.call(document.querySelectorAll('[data-list]:not([data-init])'), function (r) {
      r.setAttribute('data-init', '1');
      initList(r);
    });
  }
  document.addEventListener('DOMContentLoaded', init);
  document.addEventListener('htmx:afterSettle', init);
})();
