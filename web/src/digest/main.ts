// digest page bundle: theme picker, learning mode + help-level slider, glossary
// popups/drawer, TOC filters, keyboard shortcuts, and the onboarding tour.
// Glossary data is read from the #dl-glossary / #dl-glossary-context JSON islands.
// The blocking pre-paint IIFE (theme/zoom/learning bootstrap) stays inline in the
// template. Handlers invoked from inline on* attributes are re-exposed on window at
// the end of this module (bundling otherwise scopes them out of global reach).
import '../css/digest.css'

function applyTheme(t) {
  document.documentElement.dataset.theme = t;
  try { localStorage.setItem('downlink.theme', t); } catch(e){}
  var sel = document.getElementById('nav-theme-select');
  if (sel && sel.value !== t) sel.value = t;
}
// sync the picker with the theme chosen before first paint
(function(){ var sel = document.getElementById('nav-theme-select'); if (sel) sel.value = document.documentElement.dataset.theme || 'dark'; })();

// True while the onboarding tour owns the screen; guards handlers that would otherwise close the
// learn card / glossary or steal Escape from under it (see the tour module at the end of this file).
var tourActive = false;

// Learning mode: a master on/off toggle plus a caret-opened card holding the help-level slider
// (Full → Partial → Minimal) and per-feature switches. State lives on <html> as data-learning +
// data-help-level + data-learn-{plain,glossary,define}, persisted in localStorage and applied
// before first paint. The help level (1/2/3) decides which jargon is shown: a term appears when
// level >= its tier (advanced=1, intermediate=2, beginner=3). Sub-features default on when enabled.
var LEARN_FEATURES = {
  plain:    { attr: 'learnPlain',    key: 'downlink.learn.plain',    el: 'learn-feat-plain' },
  glossary: { attr: 'learnGlossary', key: 'downlink.learn.glossary', el: 'learn-feat-glossary' },
  define:   { attr: 'learnDefine',   key: 'downlink.learn.define',   el: 'learn-feat-define' }
};
// Slider positions run left→right: Full (most help) → Partial → Minimal (least). They map to the
// underlying help level (a term shows when level >= its tier): Full=3, Partial=2, Minimal=1.
var POS_LEVEL = [3, 2, 1];
var POS_MAX = POS_LEVEL.length - 1;
var LEVEL_NAME = { 1: 'Minimal', 2: 'Partial', 3: 'Full' };
function posToLevel(pos){ return POS_LEVEL[pos]; }
function levelToPos(level){ var i = POS_LEVEL.indexOf(level); return i < 0 ? 0 : i; }
function learnSetItem(k, v){ try { localStorage.setItem(k, v); } catch(e){} }
// Master LEARNING on/off. Off clears help; on restores the saved level (default Full) and
// defaults the sub-features on.
function toggleLearning() {
  var d = document.documentElement.dataset;
  var on = d.learning === 'on';
  if (on) {
    delete d.learning;
    closeLearnMenu();
  } else {
    d.learning = 'on';
    if (d.helpLevel !== '1' && d.helpLevel !== '2' && d.helpLevel !== '3') d.helpLevel = '3';
    learnSetItem('downlink.help.level', d.helpLevel);
    Object.keys(LEARN_FEATURES).forEach(function(f){
      var cfg = LEARN_FEATURES[f];
      if (d[cfg.attr] !== 'off') d[cfg.attr] = 'on';
    });
  }
  learnSetItem('downlink.learning', on ? 'off' : 'on');
  syncLearnUI();
}
// Pick the help level (1..3) from the in-menu slider. Only reachable while learning is on.
function setHelpLevel(level) {
  var d = document.documentElement.dataset;
  d.helpLevel = String(level);
  learnSetItem('downlink.help.level', String(level));
  syncLearnUI();
}
// Advance the help level one stop forward, wrapping Minimal (max) back to Full (0).
function advanceHelpLevel() {
  var d = document.documentElement.dataset;
  var level = (parseInt(d.helpLevel, 10) === 1 || parseInt(d.helpLevel, 10) === 2) ? parseInt(d.helpLevel, 10) : 3;
  var pos = levelToPos(level);
  var next = posToLevel((pos + 1) % (POS_MAX + 1));
  setHelpLevel(next);
  flashHelpLevel(LEVEL_NAME[next] || '');
}
// Briefly show the new help-level name beside the nav dots, then fade it out.
var helpFlashTimer;
function flashHelpLevel(name) {
  var el = document.getElementById('nav-help-flash');
  if (!el) return;
  el.textContent = name;
  el.classList.add('is-shown');
  clearTimeout(helpFlashTimer);
  helpFlashTimer = setTimeout(function(){ el.classList.remove('is-shown'); }, 1100);
}
function toggleLearnFeature(f) {
  var cfg = LEARN_FEATURES[f];
  if (!cfg) return;
  var d = document.documentElement.dataset;
  var on = d[cfg.attr] === 'on';
  d[cfg.attr] = on ? 'off' : 'on';
  learnSetItem(cfg.key, on ? 'off' : 'on');
  syncLearnUI();
}
function toggleLearnMenu() {
  var card = document.getElementById('learn-card');
  if (!card) return;
  if (card.hidden) { card.hidden = false; } else { card.hidden = true; }
  var caret = document.getElementById('nav-learn-caret');
  if (caret) caret.setAttribute('aria-expanded', card.hidden ? 'false' : 'true');
}
function closeLearnMenu() {
  var card = document.getElementById('learn-card');
  if (card) card.hidden = true;
  var caret = document.getElementById('nav-learn-caret');
  if (caret) caret.setAttribute('aria-expanded', 'false');
}
// Reflect the persisted state into the toggle, the mini-dots indicator, the in-menu slider,
// caret visibility, and each feature row.
function syncLearnUI() {
  var d = document.documentElement.dataset;
  var learningOn = d.learning === 'on';
  var level = (parseInt(d.helpLevel, 10) === 1 || parseInt(d.helpLevel, 10) === 2) ? parseInt(d.helpLevel, 10) : 3;
  var pos = levelToPos(level);
  var sw = document.getElementById('nav-learn-switch');
  if (sw) sw.setAttribute('aria-checked', learningOn ? 'true' : 'false');
  // Mini-dots indicator (nav) + the in-menu slider both highlight the active stop.
  document.querySelectorAll('.nav-help-mini-dot').forEach(function(dot){
    dot.classList.toggle('is-active', parseInt(dot.dataset.pos, 10) === pos);
  });
  var slider = document.getElementById('help-slider');
  if (slider) {
    slider.setAttribute('aria-valuenow', String(level));
    slider.setAttribute('aria-valuetext', LEVEL_NAME[level] || 'Full');
  }
  document.querySelectorAll('.help-slider-dot').forEach(function(dot){
    dot.classList.toggle('is-active', parseInt(dot.dataset.pos, 10) === pos);
  });
  var knob = document.getElementById('help-slider-knob');
  // Match the dots' 5px inset so the knob lands exactly on each stop.
  if (knob) knob.style.left = 'calc(5px + (100% - 10px) * ' + (pos / POS_MAX) + ')';
  // Caret visibility is CSS-driven (html[data-learning="on"]); just reset its menu state when off.
  var caret = document.getElementById('nav-learn-caret');
  if (caret && !learningOn) caret.setAttribute('aria-expanded', 'false');
  Object.keys(LEARN_FEATURES).forEach(function(f){
    var cfg = LEARN_FEATURES[f];
    var el = document.getElementById(cfg.el);
    if (el) el.setAttribute('aria-checked', d[cfg.attr] === 'on' ? 'true' : 'false');
  });
  // The glossary drawer is only reachable under Learning + the Glossary feature; close it
  // (and reset its toggle) the moment either gate is removed so it can't be left orphaned open.
  if (!(learningOn && d.learnGlossary === 'on')) closeGlossaryPanel();
}
// Right-side glossary drawer (gated by Learning + Glossary feature via CSS; closed here when off).
function toggleGlossaryPanel() {
  var p = document.getElementById('glossary-panel');
  if (!p) return;
  var open = p.classList.toggle('is-open');
  var btn = document.getElementById('glossary-panel-toggle');
  if (btn) btn.setAttribute('aria-expanded', open ? 'true' : 'false');
  if (open) { var s = document.getElementById('glossary-panel-search'); if (s) s.focus(); }
}
// Live, case-insensitive filter over each glossary entry (term + type + definition).
function filterGlossary(q) {
  q = (q || '').trim().toLowerCase();
  var panel = document.getElementById('glossary-panel');
  if (!panel) return;
  var d = document.documentElement.dataset;
  var level = d.learning === 'on' ? (parseInt(d.helpLevel, 10) || 0) : 0;
  var entries = panel.querySelectorAll('.glossary-panel-entry');
  var shown = 0;
  entries.forEach(function (el) {
    var inLevel = (parseInt(el.dataset.lvl, 10) || 2) <= level;
    var match = inLevel && (!q || el.textContent.toLowerCase().indexOf(q) !== -1);
    el.hidden = !match;
    if (match) shown++;
  });
  var empty = document.getElementById('glossary-panel-empty');
  if (empty) empty.hidden = shown !== 0;
}
function closeGlossaryPanel() {
  var p = document.getElementById('glossary-panel');
  if (p) p.classList.remove('is-open');
  var btn = document.getElementById('glossary-panel-toggle');
  if (btn) btn.setAttribute('aria-expanded', 'false');
}
syncLearnUI();
// In-menu help-level slider: click/drag jumps to the nearest stop, the keyboard nudges. Every
// path funnels into setHelpLevel. Positions run Full (left) → Minimal (right); see POS_LEVEL.
(function(){
  var slider = document.getElementById('help-slider');
  if (!slider) return;
  function currentPos(){
    var d = document.documentElement.dataset;
    var level = (parseInt(d.helpLevel, 10) === 1 || parseInt(d.helpLevel, 10) === 2) ? parseInt(d.helpLevel, 10) : 3;
    return levelToPos(level);
  }
  function posFromEvent(e){
    var r = slider.getBoundingClientRect();
    var pad = 5; // matches the dots' inset
    var span = r.width - 2 * pad;
    var frac = span > 0 ? (e.clientX - r.left - pad) / span : 0;
    return Math.max(0, Math.min(POS_MAX, Math.round(frac * POS_MAX)));
  }
  var dragging = false;
  slider.addEventListener('pointerdown', function(e){
    dragging = true;
    try { slider.setPointerCapture(e.pointerId); } catch(_){}
    var pos = posFromEvent(e);
    if (pos === currentPos()) advanceHelpLevel(); // clicking the knob cycles forward (wrap)
    else setHelpLevel(posToLevel(pos));           // clicking elsewhere keeps snap-to-nearest
    e.preventDefault();
  });
  slider.addEventListener('pointermove', function(e){ if (dragging) setHelpLevel(posToLevel(posFromEvent(e))); });
  slider.addEventListener('pointerup', function(){ dragging = false; });
  slider.addEventListener('pointercancel', function(){ dragging = false; });
  slider.addEventListener('keydown', function(e){
    var cur = currentPos();
    var p = cur;
    if (e.key === 'ArrowRight' || e.key === 'ArrowDown') p = Math.min(POS_MAX, cur + 1); // toward Minimal
    else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') p = Math.max(0, cur - 1);     // toward Full
    else if (e.key === 'Home') p = 0;
    else if (e.key === 'End') p = POS_MAX;
    else return;
    e.preventDefault();
    setHelpLevel(posToLevel(p));
  });
})();
// Close the card on outside click or Escape.
document.addEventListener('click', function(e){
  if (tourActive) return; // the tour opens the card on purpose; don't let its clicks dismiss it
  var card = document.getElementById('learn-card');
  if (!card || card.hidden) return;
  if (e.target.closest('#nav-learn')) return;
  closeLearnMenu();
});
document.addEventListener('keydown', function(e){ if (e.key === 'Escape' && !tourActive) { closeLearnMenu(); closeGlossaryPanel(); } });

function toggleTocRow(inner) {
  var row = inner.closest('.toc-row-wrap');
  var body = row.querySelector('.toc-row-body');
  var chevron = row.querySelector('.toc-row-chevron');
  if (!body) return;
  var isOpen = body.classList.contains('open');
  body.classList.toggle('open', !isOpen);
  row.classList.toggle('open', !isOpen);
  if (chevron) chevron.classList.toggle('open', !isOpen);
}

// For cluster summary clicks: toggle the canonical body separately from the <details> open/close.
function handleClusterClick(event, summary) {
  var row = summary.closest('.toc-row-wrap');
  var canonBody = row.querySelector('.toc-row-body');
  var chevron = row.querySelector('.toc-row-chevron');
  if (!canonBody) return;
  // Only toggle canon body when clicking on the title/score area, not the cluster chevron.
  if (event.target.closest('.toc-cluster-chevron')) return;
  event.preventDefault();
  var details = summary.closest('details');
  var isOpen = canonBody.classList.contains('open');
  canonBody.classList.toggle('open', !isOpen);
  row.classList.toggle('open', !isOpen);   // pop the card out with the body
  if (chevron) chevron.classList.toggle('open', !isOpen);
  // Also toggle the cluster members <details> together with the body.
  if (details) details.open = !isOpen;
}

// Toggle an individual cluster member's body.
function toggleTocMember(titleEl, bodyId) {
  var body = document.getElementById(bodyId);
  if (!body) return;
  body.classList.toggle('open');
}

// Reveal/hide an article's WHY justification body on demand.
function toggleWhy(label) {
  var body = label.nextElementSibling;
  if (!body) return;
  var isOpen = body.classList.toggle('open');
  label.classList.toggle('open', isOpen);
}

function togglePlain(label) {
  var body = label.nextElementSibling;
  if (!body) return;
  var isOpen = body.classList.toggle('open');
  label.classList.toggle('open', isOpen);
}

function toggleOverview(btn) {
  var grid = document.getElementById('overview-grid');
  var chev = btn.querySelector('.overview-chevron');
  var hidden = grid.style.display === 'none';
  grid.style.display = hidden ? '' : 'none';
  chev.style.transform = hidden ? 'rotate(180deg)' : '';
}

function switchTab(btn, panelId) {
  var container = btn.closest('.toc-row-body') || btn.closest('.article-body');
  if (!container) return;
  container.querySelectorAll('.tab-btn').forEach(function(b){ b.classList.remove('active'); });
  container.querySelectorAll('.tab-panel').forEach(function(p){ p.classList.remove('active'); });
  btn.classList.add('active');
  var panel = document.getElementById(panelId);
  if (panel) panel.classList.add('active');
}

var curPriority = 'all', curCategory = 'all', curTags = [];

function applyFilters() {
  document.querySelectorAll('.toc-row-wrap').forEach(function(row){
    var okP = curPriority === 'all' || row.dataset.priority === curPriority;
    var okC = curCategory === 'all' || row.dataset.category === curCategory;
    var rowTags = (row.dataset.tags || '').split(' ');
    var okT = curTags.length === 0 || curTags.some(function(t){ return rowTags.indexOf(t) >= 0; });
    row.style.display = (okP && okC && okT) ? '' : 'none';
  });
  // Hide any group whose rows are all filtered out.
  document.querySelectorAll('.toc-group').forEach(function(g){
    var any = Array.prototype.some.call(g.querySelectorAll('.toc-row-wrap'), function(r){
      return r.style.display !== 'none';
    });
    g.style.display = any ? '' : 'none';
  });
}

function setFilter(btn) {
  var f = btn.dataset.filter;
  document.querySelectorAll('.filter-btn[data-filter]').forEach(function(b){
    b.className = 'filter-btn';
    if (b.dataset.filter === f) {
      if (f === 'all') b.classList.add('active-all');
      else if (f === 'must') b.classList.add('active-must');
      else if (f === 'should') b.classList.add('active-should');
      else if (f === 'may') b.classList.add('active-may');
    }
  });
  curPriority = f;
  applyFilters();
}

function syncTagUI() {
  document.querySelectorAll('.tag-pill, .meta-tag[data-tag]').forEach(function(el){
    el.classList.toggle('active', curTags.indexOf(el.dataset.tag) >= 0);
  });
}
function toggleTag(tag) {
  var i = curTags.indexOf(tag);
  if (i >= 0) curTags.splice(i, 1); else curTags.push(tag);
  syncTagUI();
  applyFilters();
}

// Glossary: highlighted jargon/entity words in the prose open a plain-language definition in a
// persistent bottom-left popup. The lookup is keyed by a normalized form of the term (lowercase,
// strip leading '#', collapse whitespace/hyphen runs to a single space) — this MUST match
// NormalizeGlossaryKey in pkg/models/glossary.go and the highlight regexp in notification/html.go.
// Both the highlight styling and the popup are gated by Learning mode + the Definitions feature
// (html[data-learning="on"][data-learn-define="on"]). The top tag pills/chips keep their own filter
// behavior (toggleTag).
var GLOSSARY = dlReadJSON('dl-glossary');
var CONTEXT = dlReadJSON('dl-glossary-context');
function dlReadJSON(id) {
  var el = document.getElementById(id);
  if (!el) return {};
  try { return JSON.parse(el.textContent || '{}') || {}; } catch (e) { return {}; }
}
function glossaryKey(s){ return s.trim().toLowerCase().replace(/[^a-z0-9]+/g,' ').trim(); }
// Tag each highlighted term with its help tier (1=advanced, 2=intermediate, 3=beginner) so the
// CSS help-level filter can reveal/hide it. Unknown terms default to the middle tier.
document.querySelectorAll('mark.tag-hl').forEach(function(m){
  var e = GLOSSARY[glossaryKey(m.textContent)];
  m.dataset.lvl = (e && e.lvl) ? e.lvl : 2;
});
var currentMark = null;
function openGlossaryPopup(term, def, type, ctx){
  document.getElementById('glossary-popup-term').textContent = term;
  document.getElementById('glossary-popup-def').textContent = def;
  var typeEl = document.getElementById('glossary-popup-type');
  if (type) { typeEl.textContent = type; typeEl.hidden = false; } else { typeEl.hidden = true; }
  var ctxEl = document.getElementById('glossary-popup-context');
  if (ctx) { document.getElementById('glossary-popup-context-text').textContent = ctx; ctxEl.hidden = false; } else { ctxEl.hidden = true; }
  var p = document.getElementById('glossary-popup');
  // Replay the enter transition each time the content changes.
  p.hidden = false;
  p.classList.add('is-enter');
  requestAnimationFrame(function(){ p.classList.remove('is-enter'); });
}
function closeGlossaryPopup(){
  var p = document.getElementById('glossary-popup');
  if (p) p.hidden = true;
  currentMark = null;
}
document.addEventListener('click', function(e){
  var m = e.target.closest('mark.tag-hl');
  if (!m) {
    var popup = document.getElementById('glossary-popup');
    if (popup && !popup.hidden && !popup.contains(e.target)) closeGlossaryPopup();
    var panel = document.getElementById('glossary-panel');
    var ptoggle = document.getElementById('glossary-panel-toggle');
    if (panel && panel.classList.contains('is-open') && !panel.contains(e.target) && !(ptoggle && ptoggle.contains(e.target))) closeGlossaryPanel();
    return;
  }
  e.stopPropagation(); // don't bubble into row/tab handlers
  var dd = document.documentElement.dataset;
  if (dd.learning !== 'on' || dd.learnDefine !== 'on') return; // definitions disabled by Learning toggle
  var key = glossaryKey(m.textContent);
  var entry = GLOSSARY[key];
  if (!entry) return;
  var level = parseInt(dd.helpLevel, 10) || 0;
  if ((entry.lvl || 2) > level) return; // term hidden at the current help level
  if (m === currentMark) { closeGlossaryPopup(); return; }
  // Per-article context: find which article body this mark sits in (summary marks have none).
  var ctx = '';
  var host = m.closest('[data-article-id]');
  if (host) { var byArt = CONTEXT[host.dataset.articleId]; if (byArt) ctx = byArt[key] || ''; }
  currentMark = m;
  openGlossaryPopup(m.textContent.trim(), entry.def, entry.type || '', ctx);
});
document.addEventListener('keydown', function(e){ if (e.key === 'Escape') closeGlossaryPopup(); });

function setCategoryFilter(btn) {
  document.querySelectorAll('.cat-btn').forEach(function(b){ b.classList.toggle('active', b === btn); });
  curCategory = btn.dataset.cat;   // 'all' = no category filter
  applyFilters();
}

// keyboard: 'e' expand all, 'c' collapse all
document.addEventListener('keydown', function(e) {
  if (e.ctrlKey || e.altKey || e.metaKey) return;
  var t = e.target;
  if (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA') return;
  if (e.key === 'e') {
    document.querySelectorAll('.toc-row-wrap:not(.open)').forEach(function(row){
      var body = row.querySelector('.toc-row-body');
      var chevron = row.querySelector('.toc-row-chevron');
      if (body) { body.classList.add('open'); row.classList.add('open'); if (chevron) chevron.classList.add('open'); }
    });
  }
  if (e.key === 'c') {
    document.querySelectorAll('.toc-row-wrap.open').forEach(function(row){
      var body = row.querySelector('.toc-row-body');
      var chevron = row.querySelector('.toc-row-chevron');
      if (body) { body.classList.remove('open'); row.classList.remove('open'); if (chevron) chevron.classList.remove('open'); }
    });
  }
});

// Non-primary reports are pre-collapsed server-side (.report-hidden) with a pre-rendered
// "+N more" button; this just toggles them and swaps the label.
function toggleReports(btn) {
  var collapsed = btn.textContent.charAt(0) === '+';
  btn.closest('.report-list').querySelectorAll('.report-item').forEach(function(i){
    if (!i.querySelector('.report-primary')) i.classList.toggle('report-hidden', !collapsed);
  });
  btn.textContent = collapsed ? '↑ collapse' : btn.dataset.more;
}

// hide the right-fade on tag lines when scrolled flush to the right edge
(function() {
  function updateTagFade(el) {
    el.classList.toggle('at-end', el.scrollLeft + el.clientWidth >= el.scrollWidth - 1);
  }
  document.querySelectorAll('.toc-row-tags').forEach(function(el) {
    updateTagFade(el);
    el.addEventListener('scroll', function() { updateTagFade(el); }, { passive: true });
  });
})();

function toggleMoreTags(btn) {
  var cloud = btn.closest('.toc-tag-cloud');
  var collapsed = btn.textContent.charAt(0) === '+';
  cloud.classList.toggle('tags-expanded', collapsed);   /* lift/restore the mobile "first 12 only" cap */
  cloud.querySelectorAll('.tag-pill[data-tag]').forEach(function(p, i) {
    if (i >= 5) p.classList.toggle('tag-hidden', !collapsed);
  });
  btn.textContent = collapsed ? '− less' : btn.dataset.more;
}

// First-visit onboarding tour. A dim overlay with a moving spotlight walks new readers through the
// Learning toggle, the help-level slider, and the glossary, after a one-line "what this is". Shown
// once (downlink.onboarded) and replayable from the footer. It only *previews* the learning UI by
// flipping the <html> data-* flags while running and restores them on exit, so the reader's saved
// preferences are never changed.
(function(){
  var overlay = document.getElementById('tour');
  if (!overlay) return; // only rendered on Learning pages
  var spot = document.getElementById('tour-spotlight');
  var card = document.getElementById('tour-card');
  var elStep = document.getElementById('tour-step');
  var elTitle = document.getElementById('tour-title');
  var elBody = document.getElementById('tour-body');
  var elDots = document.getElementById('tour-dots');
  var btnBack = document.getElementById('tour-back');
  var btnNext = document.getElementById('tour-next');

  // reveal: flip the learning flags on so the step's control is visible. openCard: also open the
  // caret menu (the help slider lives inside it). A step with no sel is a centered, anchorless card.
  var STEPS = [
    { title: 'Welcome to DOWNLINK', body: 'DOWNLINK gives you a fast read on what shipped, broke, or matters across your feeds. Use it to scan and triage. It points you at what to read; it is not a substitute for the articles themselves.' },
    { sel: '#nav-learn-switch', reveal: true, title: 'Learning mode', body: 'Flip this on for newcomer help: plain-language summaries, click-to-define jargon, and a glossary. Turn it back off once the terms are familiar.' },
    { sel: '#help-slider', reveal: true, openCard: true, title: 'How much to explain', body: 'Slide between Full and Minimal to set how many terms get explained. Full explains nearly everything; Minimal only the rare stuff.' },
    { sel: '#nav-help-mini', reveal: true, title: 'Quick level switch', body: 'These three dots in the nav show the current help level. Click them to cycle Full → Partial → Minimal without opening the menu.' },
    { sel: '#glossary-panel-toggle', reveal: true, title: 'Glossary', body: 'This tab opens a searchable list of every term in the digest, each written in plain words.' }
  ];

  var idx = 0, active = false, saved = null;

  function snapshot(){
    var d = document.documentElement.dataset;
    return { learning: d.learning, helpLevel: d.helpLevel, glossary: d.learnGlossary };
  }
  function restore(s){
    var d = document.documentElement.dataset;
    if (s.learning) d.learning = s.learning; else delete d.learning;
    if (s.helpLevel) d.helpLevel = s.helpLevel; else delete d.helpLevel;
    if (s.glossary) d.learnGlossary = s.glossary; else delete d.learnGlossary;
    syncLearnUI();
  }
  // Temporarily reveal the learning UI (no localStorage writes) so its controls can be spotlighted.
  function preview(){
    var d = document.documentElement.dataset;
    d.learning = 'on';
    if (d.helpLevel !== '1' && d.helpLevel !== '2' && d.helpLevel !== '3') d.helpLevel = '3';
    d.learnGlossary = 'on';
    syncLearnUI();
  }

  function targetRect(sel){
    var t = sel && document.querySelector(sel);
    if (!t) return null;
    var r = t.getBoundingClientRect();
    return (r.width === 0 && r.height === 0) ? null : r;
  }
  function clamp(v, lo, hi){ return Math.max(lo, Math.min(hi, v)); }
  function placeSpotlight(r){
    var pad = 6;
    spot.classList.remove('is-centered');
    spot.style.top = (r.top - pad) + 'px';
    spot.style.left = (r.left - pad) + 'px';
    spot.style.width = (r.width + pad * 2) + 'px';
    spot.style.height = (r.height + pad * 2) + 'px';
  }
  function placeCard(r){
    var m = 14, cw = card.offsetWidth, ch = card.offsetHeight, vw = innerWidth, vh = innerHeight;
    var top, left;
    if (!r){ // centered welcome card
      left = (vw - cw) / 2; top = (vh - ch) / 2;
    } else if (r.left > vw * 0.6 && r.left - m - cw >= m){ // right-edge target → place left
      left = r.left - m - cw; top = clamp(r.top + r.height / 2 - ch / 2, m, vh - ch - m);
    } else if (vh - r.bottom >= ch + m){ // room below
      top = r.bottom + m; left = clamp(r.left + r.width / 2 - cw / 2, m, vw - cw - m);
    } else if (r.top - m - ch >= m){ // room above
      top = r.top - m - ch; left = clamp(r.left + r.width / 2 - cw / 2, m, vw - cw - m);
    } else { // fall back to the right
      left = clamp(r.right + m, m, vw - cw - m); top = clamp(r.top + r.height / 2 - ch / 2, m, vh - ch - m);
    }
    card.style.left = Math.round(left) + 'px';
    card.style.top = Math.round(top) + 'px';
  }
  function position(){
    var step = STEPS[idx];
    var r = step.sel ? targetRect(step.sel) : null;
    if (r) placeSpotlight(r); else spot.classList.add('is-centered');
    placeCard(r);
  }
  function render(){
    var step = STEPS[idx];
    elStep.textContent = 'Step ' + (idx + 1) + ' of ' + STEPS.length;
    elTitle.textContent = step.title;
    elBody.textContent = step.body;
    var dots = '';
    for (var i = 0; i < STEPS.length; i++){ dots += '<span' + (i === idx ? ' class="is-active"' : '') + '></span>'; }
    elDots.innerHTML = dots;
    btnBack.style.visibility = idx === 0 ? 'hidden' : 'visible';
    btnNext.textContent = idx === STEPS.length - 1 ? 'Done' : 'Next';
    if (step.reveal) preview();
    if (step.openCard){ var c = document.getElementById('learn-card'); if (c){ c.hidden = false; var car = document.getElementById('nav-learn-caret'); if (car) car.setAttribute('aria-expanded', 'true'); } }
    else closeLearnMenu();
    position();
  }
  function start(){
    if (active) return;
    active = true; tourActive = true;
    saved = snapshot();
    idx = 0;
    overlay.hidden = false;
    render();
    btnNext.focus();
    addEventListener('resize', position); addEventListener('scroll', position, true);
  }
  function end(){
    active = false; tourActive = false;
    overlay.hidden = true;
    closeLearnMenu();
    if (saved) restore(saved);
    saved = null;
    removeEventListener('resize', position); removeEventListener('scroll', position, true);
    try { localStorage.setItem('downlink.onboarded', '1'); } catch(e){}
  }
  function next(){ if (idx < STEPS.length - 1){ idx++; render(); } else end(); }
  function back(){ if (idx > 0){ idx--; render(); } }

  btnNext.addEventListener('click', next);
  btnBack.addEventListener('click', back);
  document.getElementById('tour-skip').addEventListener('click', end);
  overlay.addEventListener('keydown', function(e){
    if (e.key === 'Escape'){ e.preventDefault(); end(); }
    else if (e.key === 'ArrowRight'){ e.preventDefault(); next(); }
    else if (e.key === 'ArrowLeft'){ e.preventDefault(); back(); }
  });

  // Footer "tour" link replays it on demand.
  window.startTour = start;

  // Auto-show once for first-time visitors.
  try { if (!localStorage.getItem('downlink.onboarded')) start(); } catch(e){}
})();

// Inline on* attributes in the server-rendered markup call these by name, so they
// must live on the global object once the script is bundled into a module scope.
// (startTour is exposed above from inside the tour module.)
Object.assign(window, {
  applyTheme, advanceHelpLevel, closeGlossaryPanel, closeGlossaryPopup,
  handleClusterClick, setCategoryFilter, setFilter, switchTab,
  toggleGlossaryPanel, toggleLearnFeature, toggleLearning, toggleLearnMenu,
  toggleMoreTags, toggleOverview, togglePlain, toggleReports, toggleTag,
  toggleTocMember, toggleTocRow, toggleWhy, filterGlossary,
});

