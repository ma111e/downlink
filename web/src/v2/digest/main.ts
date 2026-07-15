// v2 digest bundle: the master-detail reader. The sidebar TOC (server-rendered) selects
// which article detail pane is shown; tabs, priority filters, the intelligence brief, the
// justification/duplicate toggles, learning mode, and the glossary popup are all wired here.
// Article data + glossary islands are server-rendered; this only drives interactivity.
// Inline on* handlers in the markup call the v2* functions, re-exposed on window at the end.
import '../../css/v2/digest.css'

var THEME_KEY = 'downlink.theme';
var LEARN_KEY = 'downlink.learning';
var HELP_KEY = 'downlink.help.level';

// Set while the onboarding tour owns the screen so the learn-card / glossary handlers below
// don't dismiss what the tour is spotlighting. The tour IIFE (bottom of file) toggles it.
var tourActive = false;

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
// The swap mimics the cross-document navigation effect: a same-document view transition
// scoped to the detail pane (named just for the transition so the page-level transition
// is unaffected) slides the old article out left and the new one in from the right.
// Skipped for the load-time selection, same-row clicks, reduced motion, and browsers
// without startViewTransition (which swap instantly, like unsupported navigation).
function v2Select(id: string) {
  var pane = document.querySelector('.v2-detail-pane') as HTMLElement | null;
  var vt = (document as any).startViewTransition;
  if (!hasSelected || id === selectedId || !vt || !pane ||
      document.documentElement.dataset.anim === 'off' ||
      window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
    v2ApplySelect(id);
    return;
  }
  // The cross-document entrance flag scopes the root page-slide; if it lingers into
  // this swap (entrance still running, or its cleanup never fired) the whole page —
  // TOC included — would slide with the article. Clear it before the capture.
  delete document.documentElement.dataset.vt;
  pane.style.viewTransitionName = 'v2-detail';
  var t = vt.call(document, function () { v2ApplySelect(id); });
  var unname = function () { pane.style.viewTransitionName = ''; };
  t.finished.then(unname, unname);
}
function v2ApplySelect(id: string) {
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
  // Cascade budget for the re-scan reveal below: rows past the cap appear instantly.
  var reveal = 0;
  document.querySelectorAll('.v2-toc-item').forEach(function (el) {
    var e = el as HTMLElement;
    var okP = !curPrio || e.dataset.priority === curPrio;
    var okC = curCategory === 'all' || e.dataset.category === curCategory;
    var rowTags = (e.dataset.tags || '').split(' ');
    var okT = curTags.length === 0 || curTags.some(function (t) { return rowTags.indexOf(t) >= 0; });
    // Settings-level gate: the Promotions & announcements toggle hides these
    // categories entirely, on top of whatever filters are active.
    var okPromo = !v2PromoOff() || PROMO_CATS.indexOf(e.dataset.category || '') < 0;
    e.hidden = !(okP && okC && okT && okPromo);
    // Re-trigger the .v2-reveal cascade (CSS animation keyed on --i) so a filter change
    // reads as the list re-scanning. Only filters run this; j/k selection stays instant.
    e.classList.remove('v2-reveal');
    if (!e.hidden && reveal < 12) {
      e.style.setProperty('--i', String(reveal++));
      void e.offsetWidth; // force reflow so re-adding the class restarts the animation
      e.classList.add('v2-reveal');
    }
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

// Collapsed layout: pills fill one row at full width; whatever overflows the first row is
// hidden and folded into a "+N more" button. The cutoff is width-based, so it must be
// measured after render (and re-measured on resize) rather than fixed to a tag count.
function v2LayoutTags(cloud: Element) {
  if (cloud.classList.contains('is-expanded')) return; // expanded shows everything
  var pills = Array.prototype.slice.call(
    cloud.querySelectorAll('.v2-tag-pill[data-tag]')
  ) as HTMLElement[];
  var more = cloud.querySelector('.v2-tag-more') as HTMLElement | null;
  if (!pills.length || !more) return;

  // Reset to a clean measurable state: all pills visible, more button present.
  pills.forEach(function (p) { p.classList.remove('tag-hidden'); });
  more.hidden = false;

  var hiddenCount = function () {
    return pills.filter(function (p) { return p.classList.contains('tag-hidden'); }).length;
  };
  // Keep the button carrying its real label while measuring — an empty placeholder is
  // narrower than "+N more", so measuring empty then filling it in would let it wrap below.
  var relabel = function () {
    var n = hiddenCount();
    more!.textContent = n > 0 ? '+' + n + ' more' : '';
  };

  var top = pills[0].offsetTop;
  // Hide any pill that wrapped onto a later row.
  pills.forEach(function (p) { if (p.offsetTop > top) p.classList.add('tag-hidden'); });
  relabel();
  // The more button must itself sit on row 1; if it (with its label) wrapped, drop trailing
  // visible pills one at a time, re-labelling after each so the width we test is the final one.
  for (var i = pills.length - 1; i >= 0 && more.offsetTop > top; i--) {
    if (pills[i].classList.contains('tag-hidden')) continue;
    pills[i].classList.add('tag-hidden');
    relabel();
  }

  if (hiddenCount() === 0) {
    more.hidden = true;
    more.textContent = '';
    return;
  }
  more.dataset.more = more.textContent || '';
}

// "+N more" / "− less": expand to show every tag (wrapping to multiple rows), or collapse
// back to a single row and recompute the overflow count.
function v2ToggleMoreTags(btn: HTMLElement) {
  var cloud = btn.closest('.v2-tagcloud');
  if (!cloud) return;
  var expanded = cloud.classList.toggle('is-expanded');
  if (expanded) {
    cloud.querySelectorAll('.v2-tag-pill.tag-hidden').forEach(function (p) {
      p.classList.remove('tag-hidden');
    });
    btn.textContent = '− less';
  } else {
    v2LayoutTags(cloud);
  }
}

// Lay out every tagcloud on load, and re-measure (while collapsed) on resize.
(function () {
  function layoutAll() {
    document.querySelectorAll('.v2-tagcloud').forEach(v2LayoutTags);
  }
  var t: number | undefined;
  addEventListener('resize', function () {
    clearTimeout(t);
    t = setTimeout(layoutAll, 120) as unknown as number;
  });
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', layoutAll);
  } else {
    layoutAll();
  }
})();

// "+N more" / "− less": non-primary reports stay hidden until the toggle expands the card list.
function v2ToggleReports(btn: HTMLElement) {
  var wrap = btn.closest('.v2-reports');
  if (!wrap) return;
  var expanded = wrap.classList.toggle('is-expanded');
  btn.classList.toggle('on', expanded); // rotates the caret via the generic .on rule
  var label = btn.querySelector('.v2-report-more-label');
  if (label) label.textContent = expanded ? '− less' : (btn.dataset.more || '+ more');
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
// State lives on <html> as data-learning + data-help-level + data-learn-{plain,glossary},
// persisted in localStorage and applied before first paint (see the head pre-paint script).
// Glossary gates the inline highlight + click-to-define popup; Plain words gates the
// "In plain words" block. Sub-features default on when Learning is enabled.
var LEARN_FEATURES: Record<string, { attr: string; key: string; el: string }> = {
  plain:    { attr: 'learnPlain',    key: 'downlink.learn.plain',    el: 'learn-feat-plain' },
  glossary: { attr: 'learnGlossary', key: 'downlink.learn.glossary', el: 'learn-feat-glossary' }
};
// Slider positions run left→right: Full (most help) → Partial → Minimal (least), mapping to the
// help level (a term shows when level >= its tier): Full=3, Partial=2, Minimal=1.
var POS_LEVEL = [3, 2, 1];
var POS_MAX = POS_LEVEL.length - 1;
function posToLevel(pos: number) { return POS_LEVEL[pos]; }
function levelToPos(level: number) { var i = POS_LEVEL.indexOf(level); return i < 0 ? 0 : i; }
function learnSet(k: string, v: string) { try { localStorage.setItem(k, v); } catch (e) {} }

function v2ToggleLearning() {
  var d = document.documentElement.dataset;
  var on = d.learning === 'on';
  if (on) {
    delete d.learning;
    v2CloseLearnMenu();
  } else {
    d.learning = 'on';
    if (d.helpLevel !== '1' && d.helpLevel !== '2' && d.helpLevel !== '3') d.helpLevel = '3';
    learnSet(HELP_KEY, d.helpLevel);
    Object.keys(LEARN_FEATURES).forEach(function (f) {
      var cfg = LEARN_FEATURES[f];
      if (d[cfg.attr] !== 'off') d[cfg.attr] = 'on';
    });
  }
  learnSet(LEARN_KEY, on ? 'off' : 'on');
  syncLearn();
  syncHelp();
  if (on) v2CloseGloss();
}
// Reflect learning + each feature's persisted state into the toggle, caret, and switches.
function syncLearn() {
  var d = document.documentElement.dataset;
  var on = d.learning === 'on';
  var t = document.getElementById('learn-toggle');
  if (t) { t.setAttribute('aria-checked', on ? 'true' : 'false'); t.classList.toggle('on', on); }
  var caret = document.getElementById('learn-caret');
  if (caret && !on) caret.setAttribute('aria-expanded', 'false');
  Object.keys(LEARN_FEATURES).forEach(function (f) {
    var cfg = LEARN_FEATURES[f];
    var el = document.getElementById(cfg.el);
    if (el) el.setAttribute('aria-checked', d[cfg.attr] === 'on' ? 'true' : 'false');
  });
  // The glossary drawer is only reachable under Learning + the Glossary feature; close it
  // the moment either gate drops so it can't be left orphaned open.
  if (!(on && d.learnGlossary === 'on')) closeGlossaryPanel();
}

// ---- glossary side panel (right drawer) -------------------------------------
function toggleGlossaryPanel() {
  var p = document.getElementById('glossary-panel');
  if (!p) return;
  var open = p.classList.toggle('is-open');
  var back = document.getElementById('glossary-backdrop');
  if (back) back.classList.toggle('is-open', open);
  var btn = document.getElementById('glossary-panel-toggle');
  if (btn) btn.setAttribute('aria-expanded', open ? 'true' : 'false');
  if (open) { var s = document.getElementById('glossary-panel-search') as HTMLInputElement | null; if (s) s.focus(); }
}
function closeGlossaryPanel() {
  var p = document.getElementById('glossary-panel');
  if (p) p.classList.remove('is-open');
  var back = document.getElementById('glossary-backdrop');
  if (back) back.classList.remove('is-open');
  var btn = document.getElementById('glossary-panel-toggle');
  if (btn) btn.setAttribute('aria-expanded', 'false');
}
// Live, case-insensitive filter over each entry (term + type + definition), respecting the
// current help level: entries whose tier exceeds the level are excluded from the results.
function filterGlossary(q: string) {
  q = (q || '').trim().toLowerCase();
  var panel = document.getElementById('glossary-panel');
  if (!panel) return;
  var d = document.documentElement.dataset;
  var level = d.learning === 'on' ? (parseInt(d.helpLevel || '0', 10) || 0) : 0;
  var entries = panel.querySelectorAll('.glossary-panel-entry');
  var shown = 0;
  entries.forEach(function (el) {
    var inLevel = (parseInt((el as HTMLElement).dataset.lvl || '2', 10) || 2) <= level;
    var match = inLevel && (!q || (el.textContent || '').toLowerCase().indexOf(q) !== -1);
    (el as HTMLElement).hidden = !match;
    if (match) shown++;
  });
  var empty = document.getElementById('glossary-panel-empty');
  if (empty) (empty as HTMLElement).hidden = shown !== 0;
}

function v2ToggleLearnFeature(f: string) {
  var cfg = LEARN_FEATURES[f];
  if (!cfg) return;
  var d = document.documentElement.dataset;
  var on = d[cfg.attr] === 'on';
  d[cfg.attr] = on ? 'off' : 'on';
  learnSet(cfg.key, on ? 'off' : 'on');
  // Turning off Glossary removes the highlight + popup affordance; close any open popup.
  if (f === 'glossary' && on) v2CloseGloss();
  syncLearn();
}
function v2ToggleLearnMenu() {
  var card = document.getElementById('learn-card');
  if (!card) return;
  card.hidden = !card.hidden;
  var caret = document.getElementById('learn-caret');
  if (caret) caret.setAttribute('aria-expanded', card.hidden ? 'false' : 'true');
  if (!card.hidden) syncHelp();
}
function v2CloseLearnMenu() {
  var card = document.getElementById('learn-card');
  if (card) card.hidden = true;
  var caret = document.getElementById('learn-caret');
  if (caret) caret.setAttribute('aria-expanded', 'false');
}

// ---- settings menu (gear dropdown: theme, animations, tour) -------------------
var ANIM_KEY = 'downlink.anim';
function v2ToggleSettings() {
  var card = document.getElementById('settings-card');
  if (!card) return;
  card.hidden = !card.hidden;
  var btn = document.getElementById('settings-toggle');
  if (btn) btn.setAttribute('aria-expanded', card.hidden ? 'false' : 'true');
}
function v2CloseSettings() {
  var card = document.getElementById('settings-card');
  if (card) card.hidden = true;
  var btn = document.getElementById('settings-toggle');
  if (btn) btn.setAttribute('aria-expanded', 'false');
}
// Animations are on by default; "off" lands on <html data-anim="off"> (CSS kill switch)
// and is checked before starting view transitions. Applied pre-paint like the theme.
function v2AnimOff() { return document.documentElement.dataset.anim === 'off'; }
function v2ToggleAnim() {
  if (v2AnimOff()) delete document.documentElement.dataset.anim;
  else document.documentElement.dataset.anim = 'off';
  try { localStorage.setItem(ANIM_KEY, v2AnimOff() ? 'off' : 'on'); } catch (e) {}
  syncAnim();
}
function syncAnim() {
  var t = document.getElementById('set-anim');
  if (t) t.setAttribute('aria-checked', v2AnimOff() ? 'false' : 'true');
}
syncAnim();
// Promotions & announcements: off by default; the reader opts in to show the commercial,
// sponsored and announcement categories in the TOC (and their filter chips, via CSS on
// data-promo). The pre-paint script sets data-promo='off' unless 'on' was explicitly saved.
var PROMO_KEY = 'downlink.promo';
var PROMO_CATS = ['commercial', 'sponsored', 'announcement'];
function v2PromoOff() { return document.documentElement.dataset.promo === 'off'; }
function v2TogglePromo() {
  if (v2PromoOff()) delete document.documentElement.dataset.promo;
  else document.documentElement.dataset.promo = 'off';
  try { localStorage.setItem(PROMO_KEY, v2PromoOff() ? 'off' : 'on'); } catch (e) {}
  syncPromo();
  // If the active category filter just got hidden, fall back to ALL (re-applies filters).
  if (v2PromoOff() && PROMO_CATS.indexOf(curCategory) >= 0) v2FilterCat('all');
  else v2ApplyFilters();
}
function syncPromo() {
  var t = document.getElementById('set-promo');
  if (t) t.setAttribute('aria-checked', v2PromoOff() ? 'false' : 'true');
}
syncPromo();
if (v2PromoOff()) v2ApplyFilters(); // hide the categories before the load-time selection
// Layout switch: the "New layout" toggle is on because this page IS the new (v2) layout;
// flipping it sends the reader back to the classic layout and remembers the choice.
// window.__dlLayout is injected by the server only when more than one layout was
// published, so the toggle stays hidden (and inert) otherwise.
function v2ToggleLayout() {
  var api = (window as any).__dlLayout;
  if (api && api.has('default')) api.go('default');
}
(function () {
  var api = (window as any).__dlLayout;
  var btn = document.getElementById('set-layout');
  if (btn && api && api.has('default')) btn.hidden = false;
})();
// Close the settings card on an outside click (same pattern as the learn card).
document.addEventListener('click', function (e) {
  if (tourActive) return; // the tour opens the card on purpose; don't dismiss it
  var card = document.getElementById('settings-card');
  if (!card || card.hidden) return;
  if ((e.target as HTMLElement).closest('#nav-settings')) return;
  v2CloseSettings();
});

// Help level: 3=Full, 2=Partial, 1=Minimal (default Full). A glossary term is highlighted
// only when the level >= its tier (data-lvl). The mini-dots cycle Full -> Partial -> Minimal;
// the in-card slider offers the same choice as a draggable slider.
function v2CurrentLevel(): number {
  var v = parseInt(document.documentElement.dataset.helpLevel || '3', 10);
  return (v === 1 || v === 2) ? v : 3;
}
var HELP_NAME: Record<number, string> = { 1: 'Minimal', 2: 'Partial', 3: 'Full' };
var helpFlashTimer: any;
function v2SetHelpLevel(level: number) {
  document.documentElement.dataset.helpLevel = String(level);
  learnSet(HELP_KEY, String(level));
  syncHelp();
}
function v2CycleHelp() {
  var next = v2CurrentLevel() === 1 ? 3 : v2CurrentLevel() - 1;
  v2SetHelpLevel(next);
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
  var pos = levelToPos(v2CurrentLevel()); // Full -> 0, Partial -> 1, Minimal -> 2
  document.querySelectorAll('.v2-help-dot, .v2-help-slider-dot').forEach(function (dot) {
    dot.classList.toggle('is-active', parseInt((dot as HTMLElement).dataset.pos || '0', 10) === pos);
  });
  var slider = document.getElementById('help-slider');
  if (slider) {
    slider.setAttribute('aria-valuenow', String(v2CurrentLevel()));
    slider.setAttribute('aria-valuetext', HELP_NAME[v2CurrentLevel()] || 'Full');
  }
  var knob = document.getElementById('help-slider-knob') as HTMLElement | null;
  // Match the dots' 5px inset so the knob lands exactly on each stop.
  if (knob) knob.style.left = 'calc(5px + (100% - 10px) * ' + (pos / POS_MAX) + ')';
}
syncLearn();
syncHelp();

// In-card help-level slider: click/drag snaps to the nearest stop, the keyboard nudges.
(function () {
  var slider = document.getElementById('help-slider');
  if (!slider) return;
  function posFromEvent(e: PointerEvent) {
    var r = (slider as HTMLElement).getBoundingClientRect();
    var pad = 5; // matches the dots' inset
    var span = r.width - 2 * pad;
    var frac = span > 0 ? (e.clientX - r.left - pad) / span : 0;
    return Math.max(0, Math.min(POS_MAX, Math.round(frac * POS_MAX)));
  }
  var dragging = false;
  slider.addEventListener('pointerdown', function (e) {
    dragging = true;
    try { (slider as HTMLElement).setPointerCapture(e.pointerId); } catch (_) {}
    var pos = posFromEvent(e);
    if (pos === levelToPos(v2CurrentLevel())) v2CycleHelp(); // clicking the current stop cycles forward
    else v2SetHelpLevel(posToLevel(pos));
    e.preventDefault();
  });
  slider.addEventListener('pointermove', function (e) { if (dragging) v2SetHelpLevel(posToLevel(posFromEvent(e))); });
  slider.addEventListener('pointerup', function () { dragging = false; });
  slider.addEventListener('pointercancel', function () { dragging = false; });
  slider.addEventListener('keydown', function (e) {
    var cur = levelToPos(v2CurrentLevel());
    var p = cur;
    if (e.key === 'ArrowRight' || e.key === 'ArrowDown') p = Math.min(POS_MAX, cur + 1); // toward Minimal
    else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') p = Math.max(0, cur - 1);     // toward Full
    else if (e.key === 'Home') p = 0;
    else if (e.key === 'End') p = POS_MAX;
    else return;
    e.preventDefault();
    v2SetHelpLevel(posToLevel(p));
  });
})();
// Close the learn card on an outside click.
document.addEventListener('click', function (e) {
  if (tourActive) return; // the tour opens the card on purpose; don't let its clicks dismiss it
  var card = document.getElementById('learn-card');
  if (!card || card.hidden) return;
  if ((e.target as HTMLElement).closest('#nav-learn')) return;
  v2CloseLearnMenu();
});

// The "In plain words" block is collapsed until its label is clicked.
function v2TogglePlain(label: HTMLElement) {
  var body = label.nextElementSibling as HTMLElement | null;
  if (!body) return;
  var open = body.classList.toggle('open');
  label.classList.toggle('open', open);
}

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
    var panel = document.getElementById('glossary-panel');
    var ptoggle = document.getElementById('glossary-panel-toggle');
    if (panel && panel.classList.contains('is-open') &&
        !panel.contains(target) && !(ptoggle && ptoggle.contains(target))) closeGlossaryPanel();
    return;
  }
  var d = document.documentElement.dataset;
  // The highlight + definition popup are both gated on the Glossary feature.
  if (d.learning !== 'on' || d.learnGlossary === 'off') return;
  // Report cards wrap their context in an <a>, so a term click would otherwise follow the
  // card link. Suppress the navigation (and other click handlers) and show the popup instead.
  e.preventDefault();
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
  if (tourActive) return; // the tour owns the keyboard (Escape/arrows) while running
  var t = e.target as HTMLElement;
  if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA')) return;
  if (e.ctrlKey || e.altKey || e.metaKey) return;
  if (e.key === 'Escape') { v2CloseGloss(); v2CloseLearnMenu(); v2CloseSettings(); closeGlossaryPanel(); return; }
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

// ---- first-visit onboarding tour --------------------------------------------
// A dim overlay whose spotlight cutout frames one learning control while a floating card
// explains it. Shown once (downlink.onboarded) and replayable from the topbar "?" chip. It
// only *previews* the learning UI by flipping the <html> data-* flags while running and
// restores them on exit, so the reader's saved preferences are never changed.
(function () {
  var overlay = document.getElementById('tour');
  if (!overlay) return; // only rendered on Learning pages
  var spot = document.getElementById('tour-spotlight') as HTMLElement;
  var card = document.getElementById('tour-card') as HTMLElement;
  var elStep = document.getElementById('tour-step') as HTMLElement;
  var elTitle = document.getElementById('tour-title') as HTMLElement;
  var elBody = document.getElementById('tour-body') as HTMLElement;
  var elDots = document.getElementById('tour-dots') as HTMLElement;
  var btnBack = document.getElementById('tour-back') as HTMLElement;
  var btnNext = document.getElementById('tour-next') as HTMLElement;

  // reveal: flip the learning flags on so the step's control is visible. openCard: also open
  // the caret menu (the help slider lives inside it). openSettings: open the settings
  // dropdown so its contents can be spotlighted. A step with no sel is a centered card.
  var STEPS: Array<{ sel?: string; reveal?: boolean; openCard?: boolean; openSettings?: boolean; title: string; body: string }> = [
    { title: 'Welcome to DOWNLINK', body: 'DOWNLINK gives you a fast read on what shipped, broke, or matters across your feeds. Use it to scan and triage. It points you at what to read; it is not a substitute for the articles themselves.' },
    { sel: '#settings-card', openSettings: true, title: 'Settings', body: 'The theme, an animations switch, a toggle to hide promotional and announcement content, and this tour live behind the gear. Preferences are saved in your browser and carry across digests.' },
    { sel: '#learn-toggle', reveal: true, title: 'Learning mode', body: 'Flip this on for newcomer help: plain-language summaries, click-to-define jargon, and a glossary. Turn it back off once the terms are familiar.' },
    { sel: '#help-slider', reveal: true, openCard: true, title: 'How much to explain', body: 'Slide between Full and Minimal to set how many terms get explained. Full explains nearly everything; Minimal only the rare stuff.' },
    { sel: '#help-mini', reveal: true, title: 'Quick level switch', body: 'These three dots in the topbar show the current help level. Click them to cycle Full → Partial → Minimal without opening the menu.' },
    { sel: '#glossary-panel-toggle', reveal: true, title: 'Glossary', body: 'This tab opens a searchable list of every term in the digest, each written in plain words.' }
  ];

  var idx = 0, active = false, saved: { learning?: string; helpLevel?: string; glossary?: string } | null = null;

  function snapshot() {
    var d = document.documentElement.dataset;
    return { learning: d.learning, helpLevel: d.helpLevel, glossary: d.learnGlossary };
  }
  function restore(s: { learning?: string; helpLevel?: string; glossary?: string }) {
    var d = document.documentElement.dataset;
    if (s.learning) d.learning = s.learning; else delete d.learning;
    if (s.helpLevel) d.helpLevel = s.helpLevel; else delete d.helpLevel;
    if (s.glossary) d.learnGlossary = s.glossary; else delete d.learnGlossary;
    syncLearn(); syncHelp();
  }
  // Temporarily reveal the learning UI (no localStorage writes) so its controls can be spotlighted.
  function preview() {
    var d = document.documentElement.dataset;
    d.learning = 'on';
    if (d.helpLevel !== '1' && d.helpLevel !== '2' && d.helpLevel !== '3') d.helpLevel = '3';
    d.learnGlossary = 'on';
    syncLearn(); syncHelp();
  }
  function targetRect(sel?: string) {
    var t = sel ? document.querySelector(sel) : null;
    if (!t) return null;
    var r = t.getBoundingClientRect();
    return (r.width === 0 && r.height === 0) ? null : r;
  }
  function clamp(v: number, lo: number, hi: number) { return Math.max(lo, Math.min(hi, v)); }
  function placeSpotlight(r: DOMRect) {
    var pad = 6;
    spot.classList.remove('is-centered');
    spot.style.top = (r.top - pad) + 'px';
    spot.style.left = (r.left - pad) + 'px';
    spot.style.width = (r.width + pad * 2) + 'px';
    spot.style.height = (r.height + pad * 2) + 'px';
  }
  function placeCard(r: DOMRect | null) {
    var m = 14, cw = card.offsetWidth, ch = card.offsetHeight, vw = innerWidth, vh = innerHeight;
    var top, left;
    if (!r) { left = (vw - cw) / 2; top = (vh - ch) / 2; }
    else if (r.left > vw * 0.6 && r.left - m - cw >= m) { left = r.left - m - cw; top = clamp(r.top + r.height / 2 - ch / 2, m, vh - ch - m); }
    else if (vh - r.bottom >= ch + m) { top = r.bottom + m; left = clamp(r.left + r.width / 2 - cw / 2, m, vw - cw - m); }
    else if (r.top - m - ch >= m) { top = r.top - m - ch; left = clamp(r.left + r.width / 2 - cw / 2, m, vw - cw - m); }
    else { left = clamp(r.right + m, m, vw - cw - m); top = clamp(r.top + r.height / 2 - ch / 2, m, vh - ch - m); }
    card.style.left = Math.round(left) + 'px';
    card.style.top = Math.round(top) + 'px';
  }
  function position() {
    var step = STEPS[idx];
    var r = step.sel ? targetRect(step.sel) : null;
    if (r) placeSpotlight(r); else spot.classList.add('is-centered');
    placeCard(r);
  }
  function render() {
    var step = STEPS[idx];
    elStep.textContent = 'Step ' + (idx + 1) + ' of ' + STEPS.length;
    elTitle.textContent = step.title;
    elBody.textContent = step.body;
    var dots = '';
    for (var i = 0; i < STEPS.length; i++) { dots += '<span' + (i === idx ? ' class="is-active"' : '') + '></span>'; }
    elDots.innerHTML = dots;
    btnBack.style.visibility = idx === 0 ? 'hidden' : 'visible';
    btnNext.textContent = idx === STEPS.length - 1 ? 'Done' : 'Next';
    if (step.reveal) preview();
    if (step.openCard) { var c = document.getElementById('learn-card'); if (c) { c.hidden = false; var car = document.getElementById('learn-caret'); if (car) car.setAttribute('aria-expanded', 'true'); } }
    else v2CloseLearnMenu();
    if (step.openSettings) { var s = document.getElementById('settings-card'); if (s) { s.hidden = false; var sb = document.getElementById('settings-toggle'); if (sb) sb.setAttribute('aria-expanded', 'true'); } }
    else v2CloseSettings();
    position();
  }
  function start() {
    if (active) return;
    active = true; tourActive = true;
    saved = snapshot();
    idx = 0;
    overlay!.hidden = false;
    render();
    btnNext.focus();
    addEventListener('resize', position); addEventListener('scroll', position, true);
  }
  function end() {
    active = false; tourActive = false;
    overlay!.hidden = true;
    v2CloseLearnMenu();
    v2CloseSettings();
    if (saved) restore(saved);
    saved = null;
    removeEventListener('resize', position); removeEventListener('scroll', position, true);
    try { localStorage.setItem('downlink.onboarded', '1'); } catch (e) {}
  }
  function next() { if (idx < STEPS.length - 1) { idx++; render(); } else end(); }
  function back() { if (idx > 0) { idx--; render(); } }

  btnNext.addEventListener('click', next);
  btnBack.addEventListener('click', back);
  document.getElementById('tour-skip')!.addEventListener('click', end);
  overlay.addEventListener('keydown', function (e) {
    if (e.key === 'Escape') { e.preventDefault(); end(); }
    else if (e.key === 'ArrowRight') { e.preventDefault(); next(); }
    else if (e.key === 'ArrowLeft') { e.preventDefault(); back(); }
  });

  (window as any).startTour = start; // topbar "?" chip replays it on demand
  try { if (!localStorage.getItem('downlink.onboarded')) start(); } catch (e) {}
})();

// Inline on* handlers in the server markup call these by name.
Object.assign(window, {
  v2ApplyTheme, v2Select, v2FilterPrio, v2FilterCat, v2ToggleTag, v2ToggleMoreTags, v2Tab, v2ToggleBrief,
  v2ToggleWhy, v2ToggleDup, v2ToggleLearning, v2CycleHelp, v2CloseGloss,
  v2ToggleLearnFeature, v2ToggleLearnMenu, v2TogglePlain, v2ToggleReports,
  v2ToggleSettings, v2CloseSettings, v2ToggleAnim, v2TogglePromo, v2ToggleLayout,
  toggleGlossaryPanel, closeGlossaryPanel, filterGlossary
});
