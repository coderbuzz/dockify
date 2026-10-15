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
document.addEventListener('click', function (e) {
  if (e.target.classList && e.target.classList.contains('sheet')) e.target.close();
});
