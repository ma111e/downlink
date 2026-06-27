// reports page bundle: theme picker + search/filter/sort over the reports list.
// Report data is read from the #dl-reports JSON island. The blocking pre-paint
// theme IIFE stays inline in the template.
import '../css/reports.css'

(function() {
  var THEME_KEY = 'downlink.theme';
  var sel = document.getElementById('theme');
  if (sel) {
    var current = localStorage.getItem(THEME_KEY) || document.documentElement.dataset.theme || 'dark';
    document.documentElement.dataset.theme = current;
    sel.value = current;
    sel.addEventListener('change', function() {
      document.documentElement.dataset.theme = sel.value;
      localStorage.setItem(THEME_KEY, sel.value);
    });
  }

  var reports = readReports();
  function readReports() {
    var el = document.getElementById('dl-reports');
    if (!el) return [];
    try {
      var data = JSON.parse(el.textContent || '[]');
      return Array.isArray(data) ? data : [];
    } catch (e) { return []; }
  }
  var state = { q: '', cat: '', tags: {}, sort: 'refs' };
  var els = {
    search: document.getElementById('search'),
    sort: document.getElementById('sort'),
    catrow: document.getElementById('catrow'),
    tagrow: document.getElementById('tagrow'),
    list: document.getElementById('list'),
    count: document.getElementById('count'),
    empty: document.getElementById('empty')
  };

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function(c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  function hostOf(u) { try { return new URL(u).host.replace(/^www\./, ''); } catch (e) { return ''; } }

  // The dates a report was referenced are the publish dates of its source
  // articles (YYYY-MM-DD, lexically sortable). Returns a single date or a
  // first–last range, or '' when no source has a date.
  function refDateRange(sources) {
    var dates = (sources || []).map(function(s) { return s.publishedAt; }).filter(Boolean).sort();
    if (!dates.length) return '';
    var first = dates[0], last = dates[dates.length - 1];
    return first === last ? first : first + ' – ' + last;
  }

  // Build category + tag filter chips from the data.
  function tally(key) {
    var counts = {};
    reports.forEach(function(r) {
      if (key === 'category') {
        var c = (r.category || '').trim();
        if (c) counts[c] = (counts[c] || 0) + 1;
      } else {
        (r.tags || []).forEach(function(t) { counts[t] = (counts[t] || 0) + 1; });
      }
    });
    return Object.keys(counts).sort(function(a, b) {
      if (counts[b] !== counts[a]) return counts[b] - counts[a];
      return a.toLowerCase() < b.toLowerCase() ? -1 : 1;
    }).map(function(k) { return { name: k, count: counts[k] }; });
  }

  function buildCats() {
    tally('category').forEach(function(c) {
      var b = document.createElement('button');
      b.className = 'chip';
      b.type = 'button';
      b.innerHTML = esc(c.name) + '<span class="ct">' + c.count + '</span>';
      b.addEventListener('click', function() {
        state.cat = (state.cat === c.name) ? '' : c.name;
        els.catrow.querySelectorAll('.chip').forEach(function(x) { x.classList.remove('on'); });
        if (state.cat) b.classList.add('on');
        render();
      });
      els.catrow.appendChild(b);
    });
  }

  // Full tag list, computed once. The tag row is a typeahead: chips only appear
  // for tags matching what the user has typed (plus any currently-selected tag).
  var allTags = tally('tag');

  function renderTags() {
    els.tagrow.querySelectorAll('.chip').forEach(function(x) { x.remove(); });
    var q = state.q;
    var shown = allTags.filter(function(t) {
      if (state.tags[t.name]) return true; // keep selected tags visible/removable
      return q && t.name.toLowerCase().indexOf(q) !== -1;
    });
    shown.forEach(function(t) {
      var b = document.createElement('button');
      b.className = 'chip' + (state.tags[t.name] ? ' on' : '');
      b.type = 'button';
      b.dataset.tag = t.name;
      b.innerHTML = esc(t.name) + '<span class="ct">' + t.count + '</span>';
      b.addEventListener('click', function() { toggleTag(t.name); });
      els.tagrow.appendChild(b);
    });
    els.tagrow.style.display = shown.length ? '' : 'none';
  }

  function toggleTag(name) {
    if (state.tags[name]) delete state.tags[name]; else state.tags[name] = true;
    renderTags();
    render();
  }

  function toggleMoreTags(btn) {
    var tagContainer = btn.parentElement;
    tagContainer.classList.toggle('tags-expanded');
    if (tagContainer.classList.contains('tags-expanded')) {
      btn.textContent = '- fewer';
    } else {
      var n = 0;
      tagContainer.querySelectorAll('.tag-hidden').forEach(function() { n++; });
      btn.textContent = '+' + n + ' more';
    }
  }

  function matches(r) {
    if (state.cat && (r.category || '') !== state.cat) return false;
    var active = Object.keys(state.tags);
    if (active.length) {
      var rtags = r.tags || [];
      for (var i = 0; i < active.length; i++) { if (rtags.indexOf(active[i]) === -1) return false; }
    }
    if (state.q) {
      var hay = [r.title, r.publisher, r.category, (r.tags || []).join(' ')];
      (r.sources || []).forEach(function(s) { hay.push(s.title, s.context, s.description); });
      if (hay.join(' \n ').toLowerCase().indexOf(state.q) === -1) return false;
    }
    return true;
  }

  function sortRows(rows) {
    rows.sort(function(a, b) {
      if (state.sort === 'title') {
        return (a.title || '').toLowerCase() < (b.title || '').toLowerCase() ? -1 : 1;
      }
      if ((b.refCount || 0) !== (a.refCount || 0)) return (b.refCount || 0) - (a.refCount || 0);
      return (a.title || '').toLowerCase() < (b.title || '').toLowerCase() ? -1 : 1;
    });
    return rows;
  }

  function repHTML(r) {
    var host = hostOf(r.url);
    var cat = (r.category || '').trim();
    var allTags = r.tags || [];
    var visibleTags = allTags.slice(0, 5);
    var hiddenTags = allTags.slice(5);
    var tags = visibleTags.map(function(t) {
      return '<button class="tag' + (state.tags[t] ? ' on' : '') + '" data-tag="' + esc(t) + '" type="button">' + esc(t) + '</button>';
    }).concat(hiddenTags.map(function(t) {
      return '<button class="tag tag-hidden' + (state.tags[t] ? ' on' : '') + '" data-tag="' + esc(t) + '" type="button">' + esc(t) + '</button>';
    })).join('');
    if (hiddenTags.length > 0) {
      tags += '<button class="tag tag-show-more" type="button" onclick="toggleMoreTags(this)">+' + hiddenTags.length + ' more</button>';
    }
    var srcCount = (r.sources || []).length;
    var sources = (r.sources || []).map(function(s) {
      var date = s.publishedAt ? '<span class="srcdate">' + esc(s.publishedAt) + '</span>' : '';
      var title = s.link
        ? '<a class="srctitle" href="' + esc(s.link) + '" target="_blank" rel="noopener noreferrer">' + esc(s.title || s.link) + '</a>'
        : '<span class="srctitle">' + esc(s.title || '(unknown article)') + '</span>';
      var desc = s.description ? '<div class="srcdesc">' + esc(s.description) + '</div>' : '';
      var ctx = s.context ? '<div class="srcctx">“' + esc(s.context) + '”</div>' : '';
      return '<li class="srcitem">' + title + date + desc + ctx + '</li>';
    }).join('');

    var refLabel = (r.refCount || 0) + '× referenced';
    var refDates = refDateRange(r.sources);
    return ''
      + '<li class="rep">'
      + '<div class="rep-top">'
      +   '<a class="rep-title" href="' + esc(r.url) + '" target="_blank" rel="noopener noreferrer">' + esc(r.title || r.url) + '</a>'
      +   (r.primary ? '<span class="badge primary">primary</span>' : '')
      +   (cat ? '<span class="badge cat">' + esc(cat) + '</span>' : '')
      + '</div>'
      + '<div class="rep-meta">'
      +   (r.publisher ? '<span class="pub">' + esc(r.publisher) + '</span>' : '')
      +   (host ? '<span>' + esc(host) + '</span>' : '')
      +   '<span>' + refLabel + '</span>'
      +   (refDates ? '<span class="dates">' + esc(refDates) + '</span>' : '')
      + '</div>'
      + (tags ? '<div class="rep-tags">' + tags + '</div>' : '')
      + '<button class="srctoggle" type="button">show ' + srcCount + ' source' + (srcCount === 1 ? '' : 's') + ' ▾</button>'
      + '<ul class="sources">' + sources + '</ul>'
      + '</li>';
  }

  function render() {
    var rows = sortRows(reports.filter(matches));
    els.count.textContent = rows.length + ' of ' + reports.length + ' report' + (reports.length === 1 ? '' : 's');
    els.list.innerHTML = rows.map(repHTML).join('');
    els.empty.style.display = rows.length ? 'none' : 'block';

    // Wire per-row interactions.
    els.list.querySelectorAll('.rep').forEach(function(rep) {
      var toggle = rep.querySelector('.srctoggle');
      var sources = rep.querySelector('.sources');
      if (toggle && sources) {
        toggle.addEventListener('click', function() { sources.classList.toggle('open'); });
      }
      rep.querySelectorAll('.tag').forEach(function(tg) {
        tg.addEventListener('click', function() { toggleTag(tg.dataset.tag); });
      });
    });
  }

  els.search.addEventListener('input', function() { state.q = els.search.value.trim().toLowerCase(); renderTags(); render(); });
  els.sort.addEventListener('click', function() {
    state.sort = (state.sort === 'refs') ? 'title' : 'refs';
    els.sort.textContent = 'sort: ' + (state.sort === 'refs' ? 'most referenced' : 'title (a–z)');
    render();
  });

  // Exposed because repHTML emits onclick="toggleMoreTags(this)" into the list,
  // and bundling otherwise scopes the function out of the global namespace.
  (window as any).toggleMoreTags = toggleMoreTags;

  buildCats();
  renderTags();
  render();
})();
