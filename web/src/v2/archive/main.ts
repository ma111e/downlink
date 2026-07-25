// v2 archive-index bundle: fetches manifest.json and renders the redesigned archive —
// a date-grouped log of digests with a priority-mix bar per row, a hero for the latest
// transmission, and a search control plus j/k navigation.
// The blocking pre-paint theme IIFE stays inline in the template.
import '../../css/v2/archive-index.css'

(function () {
  var THEME_KEY = 'downlink.theme';

  var state = {
    manifest: {} as any,
    rows: [] as any[],
    flat: [] as any[],   // visible rows in render order, for j/k
    active: 0,
    query: '',
    theme: localStorage.getItem(THEME_KEY) || document.documentElement.dataset.theme || 'dark'
  };

  var els = {
    archive: document.getElementById('archive') as HTMLElement,
    hero: document.getElementById('hero') as HTMLElement,
    topMeta: document.getElementById('top-meta') as HTMLElement,
    footerTotal: document.getElementById('footer-total') as HTMLElement,
    search: document.getElementById('search') as HTMLInputElement,
    theme: document.getElementById('theme') as HTMLSelectElement
  };

  document.documentElement.dataset.theme = state.theme;
  els.theme.value = state.theme;

  var manifestURL = els.archive.getAttribute('data-manifest-url') || 'manifest.json';
  var digestBaseURL = els.archive.getAttribute('data-digest-base-url') || '';

  fetch(manifestURL, { cache: 'no-cache' }).then(function (r) {
    if (!r.ok) throw new Error('manifest fetch ' + r.status);
    return r.json();
  }).then(function (m) {
    state.manifest = m || {};
    state.rows = (state.manifest.digests || []).slice().sort(function (a, b) {
      var ta = parseTs(a.period_start || a.started_at);
      var tb = parseTs(b.period_start || b.started_at);
      return (tb ? tb.getTime() : 0) - (ta ? ta.getTime() : 0);
    });
    render();
  }).catch(function (err) {
    els.hero.innerHTML = '<div class="v2-hero-empty">manifest.json failed to load · ' + escapeHTML(String(err)) + '</div>';
    els.archive.innerHTML = '<div class="v2-empty">Make sure manifest.json sits next to index.html.</div>';
  });

  els.search.addEventListener('input', function () { state.query = els.search.value; render(); });
  els.theme.addEventListener('change', function () {
    state.theme = els.theme.value;
    document.documentElement.dataset.theme = state.theme;
    try { localStorage.setItem(THEME_KEY, state.theme); } catch (e) {}
  });

  // Settings menu (gear dropdown holding the theme select + Animations switch).
  // The pre-paint IIFE applies the persisted downlink.anim flag; this only toggles it.
  var ANIM_KEY = 'downlink.anim';
  var settingsCard = document.getElementById('settings-card') as HTMLElement | null;
  var settingsBtn = document.getElementById('settings-toggle') as HTMLElement | null;
  var animBtn = document.getElementById('set-anim') as HTMLElement | null;
  function animOff() { return document.documentElement.dataset.anim === 'off'; }
  function syncAnim() { if (animBtn) animBtn.setAttribute('aria-checked', animOff() ? 'false' : 'true'); }
  function setSettingsOpen(open: boolean) {
    if (!settingsCard || !settingsBtn) return;
    settingsCard.hidden = !open;
    settingsBtn.setAttribute('aria-expanded', open ? 'true' : 'false');
  }
  if (settingsBtn) settingsBtn.addEventListener('click', function () { setSettingsOpen(!!settingsCard && settingsCard.hidden); });
  if (animBtn) animBtn.addEventListener('click', function () {
    if (animOff()) delete document.documentElement.dataset.anim;
    else document.documentElement.dataset.anim = 'off';
    try { localStorage.setItem(ANIM_KEY, animOff() ? 'off' : 'on'); } catch (e) {}
    syncAnim();
  });
  syncAnim();
  document.addEventListener('click', function (e) {
    if (!settingsCard || settingsCard.hidden) return;
    if ((e.target as HTMLElement).closest('#nav-settings')) return;
    setSettingsOpen(false);
  });

  document.addEventListener('keydown', function (e) {
    var t = e.target as HTMLElement;
    var editing = t && ['INPUT', 'SELECT', 'TEXTAREA'].indexOf(t.tagName) >= 0;
    if (editing) { if (e.key === 'Escape') t.blur(); return; }
    if (e.ctrlKey || e.altKey || e.metaKey) return;
    if (e.key === 'Escape') { setSettingsOpen(false); return; }
    if (e.key === '/') { e.preventDefault(); els.search.focus(); return; }
    if (e.key === 'j' || e.key === 'ArrowDown') { e.preventDefault(); setActive(state.active + 1); }
    else if (e.key === 'k' || e.key === 'ArrowUp') { e.preventDefault(); setActive(state.active - 1); }
    else if (e.key === 'Enter') {
      var row = state.flat[state.active];
      if (row) window.location.href = digestURL(row.filename);
    }
  });

  function render() {
    var q = state.query.trim().toLowerCase();
    var flat: any[] = [];

    // Group by UTC day, keeping the newest-first order the rows arrived in.
    var order: string[] = [];
    var groups: Record<string, any> = {};
    state.rows.forEach(function (d) {
      if (q && !matchesQuery(d, q)) return;
      var dt = parseTs(d.period_start || d.started_at);
      var key = dt ? dt.toISOString().slice(0, 10) : 'unknown';
      if (!groups[key]) { groups[key] = { dt: dt, rows: [] }; order.push(key); }
      groups[key].rows.push(d);
    });

    // Sort within each group, newest first.
    order.forEach(function (key) {
      groups[key].rows.sort(function (a: any, b: any) {
        var ta = parseTs(a.period_start || a.started_at); var tb = parseTs(b.period_start || b.started_at);
        return (tb ? tb.getTime() : 0) - (ta ? ta.getTime() : 0);
      });
    });

    renderHero();

    if (!order.length) {
      els.archive.innerHTML = '<div class="v2-empty">no digests match — clear filters or search</div>';
      state.flat = [];
      return;
    }

    var html = order.map(function (key) {
      var g = groups[key];
      var arts = g.rows.reduce(function (s: number, r: any) { return s + (r.article_count || 0); }, 0);
      var head = '<div class="v2-daygroup">' +
        '<span class="v2-daylabel">' + escapeHTML(dayLabel(g.dt)) + '</span>' +
        '<span class="v2-dayrule"></span>' +
        '<span class="v2-daysum">' + g.rows.length + ' digest' + (g.rows.length === 1 ? '' : 's') + ' · ' + arts + ' article' + (arts === 1 ? '' : 's') + '</span></div>';
      var rows = g.rows.map(function (d: any) {
        var idx = flat.length; flat.push(d);
        return rowHTML(d, idx);
      }).join('');
      return head + rows;
    }).join('');

    els.archive.innerHTML = html;
    state.flat = flat;
    if (state.active >= flat.length) state.active = 0;
    wireRows();
    highlightActive();
  }

  function renderHero() {
    var latest = state.rows[0];
    var total = state.rows.reduce(function (s, d) { return s + (d.article_count || 0); }, 0);
    els.topMeta.textContent = state.rows.length + ' digests · ' + total.toLocaleString() + ' articles · sync ' + relDate(parseTs(latest && (latest.period_end || latest.started_at)));
    els.footerTotal.textContent = total.toLocaleString() + ' articles across ' + state.rows.length + ' transmissions';
    if (!latest) { els.hero.innerHTML = '<div class="v2-hero-empty">No digests yet.</div>'; return; }
    var dt = parseTs(latest.period_start || latest.started_at);
    var dtEnd = parseTs(latest.period_end || latest.started_at);
    var model = (latest.models && latest.models.length ? latest.models.join(' · ') : latest.model) || 'unknown';
    els.hero.innerHTML =
      '<div class="v2-hero-top">' +
      '<div class="v2-hero-main">' +
      '<div class="v2-hero-kicker">LATEST · ' + escapeHTML(windowRange(dt, dtEnd)) + ' · ' + escapeHTML(relDate(dtEnd)) + '</div>' +
      '<a class="v2-hero-title" href="' + escapeAttr(digestURL(latest.filename)) + '">' + escapeHTML(topHeadline(latest)) + '</a>' +
      '<div class="v2-hero-stats">' +
      '<span><b>' + (latest.article_count || 0) + '</b> article' + ((latest.article_count === 1) ? '' : 's') + '</span>' +
      '<span><b>' + escapeHTML(latest.time_window || '—') + '</b> window</span>' +
      '<span><b class="v2-must-num">' + (latest.must_count || 0) + '</b> must</span>' +
      '<span>' + escapeHTML(model) + '</span>' +
      '</div></div>' +
      '<div class="v2-hero-cta">' +
      '<a class="v2-btn v2-btn-primary" href="' + escapeAttr(digestURL(latest.filename)) + '">OPEN DIGEST →</a>' +
      '<a class="v2-btn" href="' + escapeAttr(swipeURL(latest.filename)) + '"><svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round" aria-hidden="true"><rect x="4.5" y="2.5" width="9" height="11" rx="1.5" transform="rotate(9 9 8)"></rect><rect x="2.5" y="2.5" width="9" height="11" rx="1.5"></rect></svg></a>' +
      '</div></div>' +
      heroList(latest);
  }

  // The latest digest's article titles grouped under priority section labels
  // (must / should / may / other), preserving manifest order within each group.
  // When no headline has a known priority the labels are dropped and the list
  // renders flat. Scrolls when it overflows.
  function heroList(d: any) {
    var headlines = d.headlines || [];
    if (!headlines.length) return '';
    var href = escapeAttr(digestURL(d.filename));
    var buckets: Record<string, any[]> = { must: [], should: [], may: [], other: [] };
    headlines.forEach(function (h: any) {
      var pri = (h.priority || '').toLowerCase();
      buckets[buckets[pri] ? pri : 'other'].push(h);
    });
    var grouped = headlines.length > buckets.other.length;
    return '<div class="v2-hero-list">' + ['must', 'should', 'may', 'other'].map(function (key) {
      var rows = buckets[key];
      if (!rows.length) return '';
      return '<div class="v2-hero-group">' +
        (grouped ? '<div class="v2-hero-group-label ' + key + '">' + key + '</div>' : '') +
        '<ul class="v2-hero-group-items">' + rows.map(function (h: any) {
          return '<li><a class="v2-hero-item" href="' + href + '">' +
            '<span class="v2-hero-item-title">' + escapeHTML(headlineText(h)) + '</span></a></li>';
        }).join('') + '</ul></div>';
    }).join('') + '</div>';
  }

  function rowHTML(d: any, idx: number) {
    var dt = parseTs(d.period_start || d.started_at);
    var win = d.time_window || '—';
    return '<a class="v2-row" data-index="' + idx + '" href="' + escapeAttr(digestURL(d.filename)) + '">' +
      '<span class="v2-row-ts">' + escapeHTML(fmtTime(dt)) + ' <span class="v2-dim">UTC</span></span>' +
      '<span class="v2-row-win">' + escapeHTML(win) + '</span>' +
      '<span class="v2-row-head">' + escapeHTML(topHeadline(d)) + '</span>' +
      priorityBar(d) +
      '<span class="v2-row-count">' + (d.article_count || 0) + ' →</span></a>';
  }

  function priorityBar(d: any) {
    var must = d.must_count || 0, should = d.should_count || 0, may = d.may_count || 0, opt = d.opt_count || 0;
    var total = must + should + may + opt || 1;
    function seg(cls: string, n: number) { return n ? '<span class="' + cls + '" style="flex:' + n + '"></span>' : ''; }
    return '<span class="v2-mix" title="MUST ' + must + ' · SHOULD ' + should + ' · MAY ' + may + ' · OPT ' + opt + '">' +
      seg('v2-mix-must', must) + seg('v2-mix-should', should) + seg('v2-mix-may', may) + seg('v2-mix-opt', opt) +
      (total === 1 && !must && !should && !may && !opt ? '<span class="v2-mix-opt" style="flex:1"></span>' : '') +
      '</span>';
  }

  function wireRows() {
    els.archive.querySelectorAll('.v2-row').forEach(function (el) {
      el.addEventListener('mouseenter', function () {
        state.active = Number((el as HTMLElement).dataset.index);
        highlightActive();
      });
    });
  }

  function setActive(index: number) {
    if (!state.flat.length) return;
    state.active = Math.max(0, Math.min(state.flat.length - 1, index));
    highlightActive();
    var el = els.archive.querySelector('[data-index="' + state.active + '"]');
    if (el) (el as HTMLElement).scrollIntoView({ block: 'nearest' });
  }

  function highlightActive() {
    els.archive.querySelectorAll('.v2-row').forEach(function (el) {
      el.classList.toggle('is-active', Number((el as HTMLElement).dataset.index) === state.active);
    });
  }

  function matchesQuery(d: any, q: string) {
    if ((d.title || '').toLowerCase().indexOf(q) >= 0) return true;
    if ((d.filename || '').toLowerCase().indexOf(q) >= 0) return true;
    if ((d.summary || '').toLowerCase().indexOf(q) >= 0) return true;
    return (d.headlines || []).some(function (h: any) { return headlineText(h).toLowerCase().indexOf(q) >= 0; });
  }

  function digestURL(filename: string) {
    var name = String(filename || '');
    if (!digestBaseURL) return name;
    return digestBaseURL.replace(/\/?$/, '/') + name.replace(/^\/+/, '');
  }
  function swipeURL(filename: string) {
    return digestURL(String(filename || '').replace(/^downlink-digest-/, 'downlink-swipe-'));
  }
  function headlineText(h: any) { return (h && h.title) || ''; }
  function topHeadline(d: any) { return d.title || (d.headlines && headlineText(d.headlines[0])) || d.filename || 'Untitled digest'; }
  function parseTs(s: any) {
    if (!s) return null;
    var m = String(s).match(/^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2})/);
    if (!m) return null;
    return new Date(Date.UTC(+m[1], +m[2] - 1, +m[3], +m[4], +m[5]));
  }
  function fmtTime(d: Date | null) { return d ? d.toUTCString().slice(17, 22) : '--:--'; }
  function fmtDate(d: Date | null) { return d ? d.toUTCString().slice(5, 16).trim() : ''; }
  function dayLabel(d: Date | null) { return d ? (pad2(d.getUTCDate()) + ' ' + monthName(d) + ' ' + d.getUTCFullYear()) : 'UNDATED'; }
  function monthName(d: Date) { return d.toUTCString().slice(8, 11).toUpperCase(); }
  function windowRange(start: Date | null, end: Date | null) {
    if (!start) return '';
    if (!end) return (fmtDate(start) + ' ' + fmtTime(start) + ' UTC').toUpperCase();
    var sameDay = start.getUTCFullYear() === end.getUTCFullYear() && start.getUTCMonth() === end.getUTCMonth() && start.getUTCDate() === end.getUTCDate();
    var left = fmtDate(start) + ' ' + fmtTime(start);
    var right = sameDay ? fmtTime(end) : fmtDate(end) + ' ' + fmtTime(end);
    return (left + ' → ' + right + ' UTC').toUpperCase();
  }
  function relDate(d: Date | null) {
    if (!d) return '—';
    var hours = Math.round((Date.now() - d.getTime()) / 36e5);
    if (hours < 1) return 'just now';
    if (hours < 24) return hours + 'h ago';
    var days = Math.round(hours / 24);
    if (days < 30) return days + 'd ago';
    return Math.round(days / 30) + 'mo ago';
  }
  function pad2(n: number) { return String(n).padStart(2, '0'); }
  function escapeHTML(v: any) {
    return String(v == null ? '' : v).replace(/[&<>"']/g, function (ch) {
      return ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' } as any)[ch];
    });
  }
  function escapeAttr(v: any) { return escapeHTML(v); }
})();
