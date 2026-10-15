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
