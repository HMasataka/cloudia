/**
 * Cloudia Admin UI — minimal vanilla JS interactions
 * Replaces htmx for lightweight partial updates via fetch API.
 */
(function () {
  'use strict';

  // ── Notification helper ──────────────────────────────────────────────────
  function notify(msg, type) {
    var el = document.getElementById('notification');
    if (!el) return;
    el.textContent = msg;
    el.className = type || 'success';
    el.style.display = 'block';
    clearTimeout(el._timer);
    el._timer = setTimeout(function () { el.style.display = 'none'; }, 3000);
  }

  // ── DELETE resource ──────────────────────────────────────────────────────
  document.addEventListener('click', function (e) {
    var btn = e.target.closest('[data-delete-url]');
    if (!btn) return;
    var url = btn.getAttribute('data-delete-url');
    var confirmMsg = btn.getAttribute('data-confirm') || 'Delete this resource?';
    if (!confirm(confirmMsg)) return;

    fetch(url, { method: 'DELETE' })
      .then(function (res) {
        if (res.ok) {
          var row = btn.closest('tr');
          if (row) row.remove();
          notify('Deleted successfully.', 'success');
        } else {
          return res.json().then(function (body) {
            notify('Delete failed: ' + (body.error || res.statusText), 'error');
          });
        }
      })
      .catch(function (err) { notify('Network error: ' + err.message, 'error'); });
  });

  // ── Load container logs ──────────────────────────────────────────────────
  document.addEventListener('click', function (e) {
    var btn = e.target.closest('[data-logs-url]');
    if (!btn) return;
    var url = btn.getAttribute('data-logs-url');
    var target = document.getElementById(btn.getAttribute('data-logs-target'));
    if (!target) return;

    btn.disabled = true;
    btn.textContent = 'Loading…';

    var lines = btn.getAttribute('data-lines') || '100';
    fetch(url + '?lines=' + lines)
      .then(function (res) { return res.json(); })
      .then(function (body) {
        target.textContent = body.logs || '(no logs)';
        btn.textContent = 'Refresh logs';
        btn.disabled = false;
      })
      .catch(function (err) {
        target.textContent = 'Error: ' + err.message;
        btn.textContent = 'Retry';
        btn.disabled = false;
      });
  });

  // ── Filter form auto-submit on select change ─────────────────────────────
  document.addEventListener('change', function (e) {
    var sel = e.target.closest('[data-auto-submit]');
    if (!sel) return;
    sel.closest('form').submit();
  });

})();
