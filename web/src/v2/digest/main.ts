// v2 digest bundle: the master-detail reader. The sidebar TOC (server-rendered) selects
// which article detail pane is shown; tabs, priority filters, the intelligence brief, the
// justification/duplicate toggles, learning mode, and the glossary popup are all wired here.
// Article data + glossary islands are server-rendered; this only drives interactivity.
// Inline on* handlers in the markup call the v2* functions, re-exposed on window at the end.
import '../../css/v2/digest.css'

var THEME_KEY = 'downlink.theme';
var LEARN_KEY = 'downlink.learning';
var HELP_KEY = 'downlink.help.level';

function v2ApplyTheme(t: string) {
  document.documentElement.dataset.theme = t;
  try { localStorage.setItem(THEME_KEY, t); } catch (e) {}
  var sel = document.getElementById('theme') as HTMLSelectElement | null;
  if (sel && sel.value !== t) sel.value = t;
}
(function () {
  var sel = document.getElementById('theme') as HTMLSelectElement | null;
  if (sel) sel.value = document.documentElement.dataset.theme || 'dark';
})();

// ---- article selection (master-detail) --------------------------------------
var items = Array.prototype.slice.call(document.querySelectorAll('.v2-toc-item, .v2-dup')) as HTMLElement[];
var selectedId: string | null = null;

function visibleItems(): HTMLElement[] {
  return items.filter(function (el) { return el.offsetParent !== null; });
}

var hasSelected = false;
function v2Select(id: string) {
  selectedId = id;
  document.querySelectorAll('.v2-detail').forEach(function (d) {
    (d as HTMLElement).hidden = (d as HTMLElement).dataset.articleId !== id;
  });
  document.querySelectorAll('.v2-toc-item, .v2-dup').forEach(function (el) {
    el.classList.toggle('is-active', (el as HTMLElement).dataset.target === id);
  });
  var pane = document.querySelector('.v2-detail-pane') as HTMLElement | null;
  if (pane) {
    // On the stacked mobile layout the detail sits below the TOC, so bring it into
    // view when the reader picks an article. On desktop (two panes) just reset the
    // pane's own scroll. The very first (load-time) selection never scrolls the page.
    if (hasSelected && window.matchMedia('(max-width: 860px)').matches) {
      pane.scrollIntoView({ behavior: 'smooth', block: 'start' });
    } else {
      pane.scrollTop = 0;
    }
  }
  hasSelected = true;
  v2CloseGloss();
}

// ---- filters (priority + category + tags, ANDed) ----------------------------
var curPrio: string | null = null;
var curCategory = 'all';
var curTags: string[] = [];

function v2ApplyFilters() {
  document.querySelectorAll('.v2-toc-item').forEach(function (el) {
    var e = el as HTMLElement;
    var okP = !curPrio || e.dataset.priority === curPrio;
    var okC = curCategory === 'all' || e.dataset.category === curCategory;
    var rowTags = (e.dataset.tags || '').split(' ');
    var okT = curTags.length === 0 || curTags.some(function (t) { return rowTags.indexOf(t) >= 0; });
    e.hidden = !(okP && okC && okT);
  });
  // Hide a group whose items are all filtered out.
  document.querySelectorAll('.v2-toc-group').forEach(function (g) {
    var any = Array.prototype.some.call(g.querySelectorAll('.v2-toc-item'), function (r) { return !(r as HTMLElement).hidden; });
    (g as HTMLElement).style.display = any ? '' : 'none';
  });
  // If the selected article was filtered out, jump to the first visible one.
  var vis = visibleItems();
  if (vis.length && (!selectedId || vis.every(function (el) { return el.dataset.target !== selectedId; }))) {
    v2Select(vis[0].dataset.target as string);
  }
}

function v2FilterPrio(prio: string) {
  curPrio = curPrio === prio ? null : prio;
  document.querySelectorAll('.v2-prio').forEach(function (b) {
    b.classList.toggle('on', (b as HTMLElement).dataset.prio === curPrio);
  });
  v2ApplyFilters();
}

function v2FilterCat(cat: string) {
  curCategory = cat;
  document.querySelectorAll('.v2-cat').forEach(function (b) {
    b.classList.toggle('on', (b as HTMLElement).dataset.cat === cat);
  });
  v2ApplyFilters();
}

function v2ToggleTag(tag: string) {
  var i = curTags.indexOf(tag);
  if (i >= 0) curTags.splice(i, 1); else curTags.push(tag);
  document.querySelectorAll('.v2-tag-pill').forEach(function (b) {
    b.classList.toggle('on', curTags.indexOf((b as HTMLElement).dataset.tag || '') >= 0);
  });
  v2ApplyFilters();
}

// "+N more" / "− less": pills past the first 5 are hidden until expanded.
function v2ToggleMoreTags(btn: HTMLElement) {
  var cloud = btn.closest('.v2-tagcloud');
  if (!cloud) return;
  var collapsed = (btn.textContent || '').charAt(0) === '+';
  cloud.querySelectorAll('.v2-tag-pill[data-tag]').forEach(function (p, i) {
    if (i >= 5) p.classList.toggle('tag-hidden', !collapsed);
  });
  btn.textContent = collapsed ? '− less' : (btn.dataset.more || '+ more');
}

// ---- tabs -------------------------------------------------------------------
function v2Tab(btn: HTMLElement, panelId: string) {
  var article = btn.closest('.v2-detail');
  if (!article) return;
  article.querySelectorAll('.v2-tab').forEach(function (b) { b.classList.remove('active'); });
  article.querySelectorAll('.v2-tabpanel').forEach(function (p) { p.classList.remove('active'); });
  btn.classList.add('active');
  var panel = document.getElementById(panelId);
  if (panel) panel.classList.add('active');
}

// ---- collapsibles -----------------------------------------------------------
function v2ToggleBrief() {
  var panel = document.getElementById('brief-panel');
  var btn = document.getElementById('brief-toggle');
  if (!panel) return;
  panel.hidden = !panel.hidden;
  if (btn) btn.classList.toggle('on', !panel.hidden);
}
function v2ToggleWhy(btn: HTMLElement) {
  var body = btn.closest('.v2-detail-actions')?.nextElementSibling as HTMLElement | null;
  if (body && body.classList.contains('v2-why')) { body.hidden = !body.hidden; btn.classList.toggle('on', !body.hidden); }
}
function v2ToggleDup(btn: HTMLElement) {
  var item = btn.closest('.v2-toc-item');
  var list = item && item.nextElementSibling as HTMLElement | null;
  if (list && list.classList.contains('v2-dup-list')) { list.hidden = !list.hidden; btn.classList.toggle('on', !list.hidden); }
}

// ---- learning + help level + glossary ---------------------------------------
function v2ToggleLearning() {
  var d = document.documentElement.dataset;
  var on = d.learning === 'on';
  if (on) {
    delete d.learning;
  } else {
    d.learning = 'on';
    if (d.helpLevel !== '1' && d.helpLevel !== '2' && d.helpLevel !== '3') d.helpLevel = '3';
  }
  try { localStorage.setItem(LEARN_KEY, on ? 'off' : 'on'); } catch (e) {}
  syncLearn();
  syncHelp();
  if (on) v2CloseGloss();
}
function syncLearn() {
  var on = document.documentElement.dataset.learning === 'on';
  var t = document.getElementById('learn-toggle');
  if (t) { t.setAttribute('aria-checked', on ? 'true' : 'false'); t.classList.toggle('on', on); }
}

// Help level: 3=Full, 2=Partial, 1=Minimal (default Full). A glossary term is highlighted
// only when the level >= its tier (data-lvl). The 3 dots cycle Full -> Partial -> Minimal;
// the active dot's position is 3 - level (level 3 -> dot 0, 2 -> 1, 1 -> 2).
function v2CurrentLevel(): number {
  var v = parseInt(document.documentElement.dataset.helpLevel || '3', 10);
  return (v === 1 || v === 2) ? v : 3;
}
var HELP_NAME: Record<number, string> = { 1: 'Minimal', 2: 'Partial', 3: 'Full' };
var helpFlashTimer: any;
function v2CycleHelp() {
  var next = v2CurrentLevel() === 1 ? 3 : v2CurrentLevel() - 1;
  document.documentElement.dataset.helpLevel = String(next);
  try { localStorage.setItem(HELP_KEY, String(next)); } catch (e) {}
  syncHelp();
  v2FlashHelp(HELP_NAME[next] || '');
}
// Briefly show the new help-level name below the dots, then fade it out.
function v2FlashHelp(name: string) {
  var el = document.getElementById('help-flash');
  if (!el) return;
  el.textContent = name;
  el.classList.add('is-shown');
  clearTimeout(helpFlashTimer);
  helpFlashTimer = setTimeout(function () { el.classList.remove('is-shown'); }, 1100);
}
function syncHelp() {
  var pos = 3 - v2CurrentLevel();
  document.querySelectorAll('.v2-help-dot').forEach(function (dot) {
    dot.classList.toggle('is-active', parseInt((dot as HTMLElement).dataset.pos || '0', 10) === pos);
  });
}
syncLearn();
syncHelp();

function dlReadJSON(id: string) {
  var el = document.getElementById(id);
  if (!el) return {};
  try { return JSON.parse(el.textContent || '{}') || {}; } catch (e) { return {}; }
}
var GLOSSARY = dlReadJSON('dl-glossary') as Record<string, any>;
var CONTEXT = dlReadJSON('dl-glossary-context') as Record<string, any>;
function glossaryKey(s: string) { return s.trim().toLowerCase().replace(/[^a-z0-9]+/g, ' ').trim(); }

// Tag each highlighted term with its help tier (1=advanced, 2=intermediate, 3=beginner) so the
// CSS help-level filter can reveal/hide it. Unknown terms default to the middle tier.
document.querySelectorAll('mark.tag-hl').forEach(function (m) {
  var e = GLOSSARY[glossaryKey((m as HTMLElement).textContent || '')];
  (m as HTMLElement).dataset.lvl = String((e && e.lvl) ? e.lvl : 2);
});

var currentMark: HTMLElement | null = null;
function v2CloseGloss() {
  var p = document.getElementById('glossary-popup');
  if (p) p.hidden = true;
  currentMark = null;
}
document.addEventListener('click', function (e) {
  var target = e.target as HTMLElement;
  var m = target.closest('mark.tag-hl') as HTMLElement | null;
  if (!m) {
    var popup = document.getElementById('glossary-popup');
    if (popup && !popup.hidden && !popup.contains(target)) v2CloseGloss();
    return;
  }
  if (document.documentElement.dataset.learning !== 'on') return;
  e.stopPropagation();
  var key = glossaryKey(m.textContent || '');
  var entry = GLOSSARY[key];
  if (!entry) return;
  if ((entry.lvl || 2) > v2CurrentLevel()) return; // term hidden at the current help level
  if (m === currentMark) { v2CloseGloss(); return; }
  var ctx = '';
  var host = m.closest('[data-article-id]') as HTMLElement | null;
  if (host) { var byArt = CONTEXT[host.dataset.articleId as string]; if (byArt) ctx = byArt[key] || ''; }
  currentMark = m;
  var termEl = document.getElementById('glossary-popup-term');
  var typeEl = document.getElementById('glossary-popup-type');
  var defEl = document.getElementById('glossary-popup-def');
  var ctxEl = document.getElementById('glossary-popup-context');
  var ctxTextEl = document.getElementById('glossary-popup-context-text');
  if (termEl) termEl.textContent = (m.textContent || '').trim();
  if (typeEl) {
    if (entry.type) { typeEl.textContent = entry.type; typeEl.hidden = false; }
    else typeEl.hidden = true;
  }
  if (defEl) defEl.textContent = entry.def;
  if (ctxEl && ctxTextEl) {
    if (ctx) { ctxTextEl.textContent = ctx; ctxEl.hidden = false; }
    else ctxEl.hidden = true;
  }
  var p = document.getElementById('glossary-popup');
  if (p) p.hidden = false;
});

// ---- keyboard ---------------------------------------------------------------
document.addEventListener('keydown', function (e) {
  var t = e.target as HTMLElement;
  if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA')) return;
  if (e.ctrlKey || e.altKey || e.metaKey) return;
  if (e.key === 'Escape') { v2CloseGloss(); return; }
  if (e.key !== 'j' && e.key !== 'k' && e.key !== 'Enter') return;
  var vis = visibleItems();
  if (!vis.length) return;
  var pos = vis.findIndex(function (el) { return el.dataset.target === selectedId; });
  if (e.key === 'Enter') {
    var cur = document.querySelector('.v2-detail:not([hidden]) .v2-btn-primary') as HTMLAnchorElement | null;
    if (cur && cur.href) window.open(cur.href, '_blank', 'noopener');
    return;
  }
  e.preventDefault();
  var next = e.key === 'j' ? Math.min(pos + 1, vis.length - 1) : Math.max(pos - 1, 0);
  if (pos < 0) next = 0;
  var el = vis[next];
  if (el) { v2Select(el.dataset.target as string); el.scrollIntoView({ block: 'nearest' }); }
});

// Select the first article on load.
(function () {
  var first = visibleItems()[0];
  if (first) v2Select(first.dataset.target as string);
})();

// Inline on* handlers in the server markup call these by name.
Object.assign(window, {
  v2ApplyTheme, v2Select, v2FilterPrio, v2FilterCat, v2ToggleTag, v2ToggleMoreTags, v2Tab, v2ToggleBrief,
  v2ToggleWhy, v2ToggleDup, v2ToggleLearning, v2CycleHelp, v2CloseGloss
});
