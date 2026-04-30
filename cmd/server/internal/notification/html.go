package notification

import (
	"bytes"
	"downlink/pkg/digestthemes"
	"downlink/pkg/models"
	"fmt"
	"html/template"
	"sort"
	"strings"

	"github.com/gomarkdown/markdown"
	mdhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

const digestHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>DOWNLINK — {{.StartedAt}}</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500;600&family=IBM+Plex+Sans:wght@300;400;500;600&display=swap" rel="stylesheet">
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

:root {
  --bg:      #0b0b0e;
  --surface: #111116;
  --surface2:#15151c;
  --border:  #1c1c25;
  --border2: #252530;
  --text:    #dddde8;
  --text2:   #a8a8be;
  --text3:   #4a4a60;
  --cyan:    oklch(74% 0.14 196);
  --must:    oklch(66% 0.15 38);
  --should:  oklch(73% 0.16 60);
  --may:     oklch(65% 0.13 250);
  --mono:    'IBM Plex Mono', 'SF Mono', 'Fira Code', Consolas, monospace;
  --sans:    'IBM Plex Sans', -apple-system, BlinkMacSystemFont, sans-serif;
  --radius:  4px;
}
{{if .ThemeOverride}}:root{ {{.ThemeOverride}} }{{end}}

html {
  background: var(--bg);
  background-image: radial-gradient(circle, #ffffff08 1px, transparent 1px);
  background-size: 24px 24px;
  color: var(--text);
  font-family: var(--sans);
  font-size: 14px;
  line-height: 1.6;
  scroll-behavior: smooth;
}
body { min-height: 100vh; max-width: 900px; margin: 0 auto; }
::selection { background: color-mix(in oklch, var(--cyan) 25%, transparent); }
::-webkit-scrollbar { width: 6px; height: 6px; }
::-webkit-scrollbar-track { background: transparent; }
::-webkit-scrollbar-thumb { background: var(--border2); border-radius: 3px; }

a { color: var(--cyan); text-decoration: none; }
a:hover { text-decoration: underline; }

/* ── animations ──────────────────────────────────── */
@keyframes target-pulse {
  0%,100% { border-color: var(--cyan); }
  50%      { border-color: var(--border); }
}

/* ── section label ───────────────────────────────── */
.section-label {
  font-family: var(--mono);
  font-size: 10px;
  letter-spacing: 0.14em;
  color: var(--text3);
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 8px;
}
.section-label::before { content: "//"; opacity: 0.5; font-size: 11px; }

/* ── nav ─────────────────────────────────────────── */
#nav {
  position: sticky;
  top: 0;
  z-index: 100;
  background: color-mix(in oklch, var(--bg) 90%, transparent);
  backdrop-filter: blur(16px);
  border-bottom: 1px solid var(--border);
  padding: 0 40px;
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.nav-logo {
  display: flex;
  align-items: center;
  gap: 0;
}
.nav-logo-down { font-family: var(--mono); font-weight: 600; font-size: 16px; letter-spacing: 0.14em; color: var(--text); }
.nav-logo-link { font-family: var(--mono); font-weight: 600; font-size: 16px; letter-spacing: 0.14em; color: var(--cyan); }
.nav-logo-ver {
  margin-left: 10px;
  font-family: var(--mono);
  font-size: 9px;
  color: var(--cyan);
  letter-spacing: 0.1em;
  border: 1px solid color-mix(in oklch, var(--cyan) 30%, transparent);
  background: color-mix(in oklch, var(--cyan) 7%, transparent);
  padding: 1px 6px;
  border-radius: 2px;
}
.nav-center {
  display: flex;
  align-items: center;
  gap: 8px;
}
.nav-date  { font-family: var(--mono); font-size: 11px; color: var(--text3); letter-spacing: 0.06em; }
.nav-right {
  display: flex;
  align-items: center;
  gap: 20px;
}
.nav-window { font-family: var(--mono); font-size: 11px; color: var(--text3); letter-spacing: 0.06em; }
.nav-count {
  font-family: var(--mono);
  font-size: 10px;
  color: var(--text2);
  letter-spacing: 0.08em;
  border: 1px solid var(--border2);
  background: var(--surface);
  padding: 3px 10px;
  border-radius: 2px;
  display: flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
}
.nav-count-dot { color: var(--should); }

/* ── provider/model switcher ─────────────────────── */
.nav-switcher {
  font-family: var(--mono);
  font-size: 10px;
  letter-spacing: 0.08em;
  color: var(--text2);
  background: var(--surface);
  border: 1px solid var(--border2);
  border-radius: 2px;
  padding: 3px 8px;
  cursor: pointer;
  text-transform: uppercase;
  white-space: nowrap;
  appearance: none;
  -webkit-appearance: none;
  background-image: linear-gradient(45deg, transparent 50%, var(--text3) 50%),
                    linear-gradient(135deg, var(--text3) 50%, transparent 50%);
  background-position: calc(100% - 10px) 50%, calc(100% - 6px) 50%;
  background-size: 4px 4px, 4px 4px;
  background-repeat: no-repeat;
  padding-right: 22px;
}
.nav-switcher:hover { border-color: var(--cyan); color: var(--text); }
.nav-switcher:focus { outline: 1px solid var(--cyan); outline-offset: 1px; }

/* ── priority badge ──────────────────────────────── */
.priority-badge {
  font-family: var(--mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.08em;
  padding: 2px 8px;
  border-radius: 2px;
  white-space: nowrap;
  flex-shrink: 0;
}
.badge-must   { color: var(--must);   border: 1px solid color-mix(in oklch, var(--must)   45%, transparent); background: color-mix(in oklch, var(--must)   12%, transparent); }
.badge-should { color: var(--should); border: 1px solid color-mix(in oklch, var(--should) 45%, transparent); background: color-mix(in oklch, var(--should) 12%, transparent); }
.badge-may    { color: var(--may);    border: 1px solid color-mix(in oklch, var(--may)    45%, transparent); background: color-mix(in oklch, var(--may)    12%, transparent); }
.badge-opt    { color: var(--text3);  border: 1px solid var(--border2); background: transparent; }

/* ── group badge ─────────────────────────────────── */
.group-badge {
  font-family: var(--mono);
  font-size: 9px;
  font-weight: 600;
  letter-spacing: 0.1em;
  width: 10px;
  height: 10px;
  border-radius: 2px;
  cursor: default;
  flex-shrink: 0;
}

/* ── score bar ───────────────────────────────────── */
.score-bar {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 60px;
  justify-content: flex-end;
  flex-shrink: 0;
}
.score-track {
  width: 32px; height: 3px;
  background: var(--border2);
  border-radius: 2px;
  overflow: hidden;
}
.score-fill { height: 100%; border-radius: 2px; }
.score-fill-high   { background: var(--must); }
.score-fill-mid    { background: var(--should); }
.score-fill-low    { background: var(--text3); }
.score-num {
  font-family: var(--mono);
  font-size: 10px;
  min-width: 20px;
  text-align: right;
}
.score-num-high { color: var(--must); }
.score-num-mid  { color: var(--should); }
.score-num-low  { color: var(--text3); }

/* ── toc ─────────────────────────────────────────── */
#toc {
  padding: 20px 40px 48px;
}
.toc-card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  overflow: hidden;
}
.toc-header {
  padding: 13px 20px;
  border-bottom: 1px solid var(--border);
}
.toc-body { padding: 8px 20px 16px; }
.toc-group { margin-bottom: 4px; }
.toc-group-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 0 8px;
  border-bottom: 1px solid var(--border);
  margin-bottom: 4px;
}
.toc-group-label {
  font-family: var(--mono);
  font-size: 9px;
  font-weight: 600;
  letter-spacing: 0.14em;
  padding: 2px 7px;
  border-radius: 2px;
  white-space: nowrap;
}
.toc-group-count { font-family: var(--mono); font-size: 9px; color: var(--text3); }
.toc-num {
  font-family: var(--mono);
  font-size: 10px;
  min-width: 20px;
  margin-top: 2px;
  flex-shrink: 0;
}
.toc-num-must   { color: var(--must); }
.toc-num-should { color: var(--should); }
.toc-num-may    { color: var(--may); }
.toc-num-opt    { color: var(--text3); }
.toc-title-link { color: var(--text); font-size: 13px; flex: 1; line-height: 1.5; cursor: pointer; }
.toc-title-link:hover { color: var(--text2); text-decoration: none; }
.toc-child-indent { color: var(--text3); font-size: 10px; margin-top: 2px; flex-shrink: 0; }
.toc-child-title  { color: var(--text2); font-size: 12px; flex: 1; line-height: 1.5; }
.toc-child-title:hover { color: var(--text); text-decoration: none; }

/* ── toc expandable row ──────────────────────────── */
.toc-row-wrap {
  position: relative;
  border-bottom: 1px solid color-mix(in oklch, var(--border) 50%, transparent);
  transition: background 0.12s;
}
.toc-row-wrap:last-child { border-bottom: none; }
.toc-row-wrap::before {
  content: "";
  position: absolute; left: 0; top: 0; bottom: 0;
  width: 3px;
  opacity: 0;
  transition: opacity 0.15s;
}
.toc-row-wrap:hover { background: var(--surface2); }
.toc-row-wrap:hover::before { opacity: 1; }
.toc-row-wrap.must-row::before   { background: var(--must); }
.toc-row-wrap.should-row::before { background: var(--should); }
.toc-row-wrap.may-row::before    { background: var(--may); }
.toc-row-inner {
  display: flex;
  gap: 12px;
  padding: 9px 8px 9px 12px;
  align-items: flex-start;
  cursor: pointer;
  user-select: none;
}
.toc-row-meta {
  display: flex;
  gap: 8px;
  margin-top: 3px;
  align-items: center;
  flex-wrap: wrap;
}
.toc-row-chevron {
  color: var(--text3);
  font-size: 13px;
  text-align: center;
  transition: transform 0.2s;
  flex-shrink: 0;
  margin-left: auto;
  padding-left: 8px;
  align-self: center;
}
.toc-row-body { display: none; border-top: 1px solid var(--border); }
.toc-row-body.open { display: block; }

/* ── overview ────────────────────────────────────── */
#overview {
  padding: 28px 40px 0;
}
.overview-card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  overflow: hidden;
}
.overview-toggle {
  all: unset;
  cursor: pointer;
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 13px 24px;
  border-bottom: 1px solid var(--border);
  box-sizing: border-box;
}
.overview-chevron {
  color: var(--text3);
  font-size: 14px;
  transition: transform 0.2s;
}
.overview-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
}
.overview-cell {
  padding: 20px 24px;
}
.overview-cell-full { grid-column: 1 / -1; }
.overview-cell-border-r { border-right: 1px solid var(--border); }
.overview-cell-border-b { border-bottom: 1px solid var(--border); }
.overview-cell-head {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-bottom: 8px;
}
.overview-tag {
  font-family: var(--mono);
  font-size: 9px;
  font-weight: 600;
  letter-spacing: 0.12em;
  padding: 1px 5px;
  border-radius: 2px;
  flex-shrink: 0;
}
.overview-tag-exec {
  color: var(--cyan);
  border: 1px solid color-mix(in oklch, var(--cyan) 35%, transparent);
  background: color-mix(in oklch, var(--cyan) 8%, transparent);
}
.overview-tag-num {
  color: var(--text3);
  border: 1px solid var(--border2);
  background: transparent;
}
.overview-title { font-family: var(--sans); font-weight: 600; font-size: 13px; color: var(--text); letter-spacing: 0.01em; }
.overview-body  { color: var(--text2); font-size: 13px; line-height: 1.75; font-weight: 400; }
.overview-body p { margin-bottom: 0.6rem; }
.overview-body p:last-child { margin-bottom: 0; }
.overview-body ul, .overview-body ol { padding-left: 1.4rem; margin: 0.5rem 0; }
.overview-body li { margin-bottom: 0.25rem; }
.overview-body strong { color: var(--text); }
.overview-body a { color: var(--cyan); }

/* ── filter bar (in toc header) ──────────────────── */
.filter-bar { display: flex; gap: 2px; }
.filter-btn {
  background: none;
  border: 1px solid transparent;
  border-radius: 2px;
  cursor: pointer;
  font-family: var(--mono);
  font-size: 9px;
  letter-spacing: 0.1em;
  font-weight: 400;
  color: var(--text3);
  padding: 4px 10px;
  transition: all 0.15s;
  display: flex;
  align-items: center;
  gap: 5px;
}
.filter-btn.active-all    { color: var(--text2);  background: color-mix(in oklch, var(--text2)  10%, transparent); border-color: color-mix(in oklch, var(--text2)  35%, transparent); font-weight: 600; }
.filter-btn.active-must   { color: var(--must);   background: color-mix(in oklch, var(--must)   10%, transparent); border-color: color-mix(in oklch, var(--must)   35%, transparent); font-weight: 600; }
.filter-btn.active-should { color: var(--should); background: color-mix(in oklch, var(--should) 10%, transparent); border-color: color-mix(in oklch, var(--should) 35%, transparent); font-weight: 600; }
.filter-btn.active-may    { color: var(--may);    background: color-mix(in oklch, var(--may)    10%, transparent); border-color: color-mix(in oklch, var(--may)    35%, transparent); font-weight: 600; }
.filter-count { opacity: 0.6; }
.source-dot {
  width: 5px; height: 5px;
  border-radius: 50%;
  flex-shrink: 0;
  display: inline-block;
}
.source-name { font-family: var(--mono); font-size: 10px; }
.meta-sep    { color: var(--text3); font-family: var(--mono); font-size: 10px; }
.meta-time   { font-family: var(--mono); font-size: 10px; color: var(--text3); }

/* ── article expanded body ───────────────────────── */
.article-body { display: none; border-top: 1px solid var(--border); }
.article-body.open { display: block; }

.analysis-strip {
  display: flex;
  gap: 1.5rem;
  flex-wrap: wrap;
  padding: 0.6rem 1.1rem;
  background: var(--surface2);
  border-bottom: 1px solid var(--border);
  font-size: 11px;
  color: var(--text2);
}
.analysis-strip strong { color: var(--text); font-weight: 500; }
.justification p { display: inline; margin: 0; }

.tabs {
  display: flex;
  background: var(--surface);
  border-bottom: 1px solid var(--border);
  overflow-x: auto;
}
.tab-btn {
  padding: 0.55rem 1rem;
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  color: var(--text3);
  cursor: pointer;
  font-family: var(--sans);
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
  transition: color .15s, border-color .15s;
  margin-bottom: -1px;
}
.tab-btn:hover { color: var(--text); }
.tab-btn.active { color: var(--cyan); border-bottom-color: var(--cyan); }
.tab-panel { display: none; padding: 1.25rem 1.4rem; }
.tab-panel.active { display: block; }

/* ── prose ───────────────────────────────────────── */
.prose { line-height: 1.8; color: var(--text2); font-size: 13.5px; }
.prose p { margin-bottom: 0.75rem; }
.prose p:last-child { margin-bottom: 0; }
.prose h1, .prose h2, .prose h3 { color: var(--text); font-weight: 600; margin: 1.1rem 0 0.4rem; font-size: 0.9rem; letter-spacing: 0.02em; }
.prose ul, .prose ol { padding-left: 1.5rem; margin-bottom: 0.75rem; }
.prose li { margin-bottom: 0.3rem; }
.prose strong { color: var(--text); font-weight: 600; }
.prose em { color: var(--text3); font-style: italic; }
.prose code { background: var(--surface2); border: 1px solid var(--border); border-radius: 3px; padding: 0.1rem 0.35rem; font-family: var(--mono); font-size: 12px; color: var(--text); }
.prose blockquote { border-left: 2px solid color-mix(in oklch, var(--cyan) 30%, transparent); padding-left: 1rem; color: var(--text3); margin: 0.75rem 0; }
.prose a { color: var(--cyan); }

.kp-list { list-style: none; display: flex; flex-direction: column; gap: 0.5rem; }
.kp-list li { display: flex; gap: 0.6rem; font-size: 13.5px; line-height: 1.6; color: var(--text2); }
.kp-list li::before { content: "–"; color: var(--cyan); flex-shrink: 0; font-weight: 600; }

.report-list { list-style: none; display: flex; flex-direction: column; gap: 0.75rem; }
.report-item {
  border: 1px solid var(--border);
  background: var(--surface2);
  border-radius: var(--radius);
  padding: 0.75rem 0.85rem;
}
.report-title { font-size: 13.5px; font-weight: 600; line-height: 1.45; color: var(--text); }
.report-title a { color: var(--text); }
.report-title a:hover { color: var(--cyan); }
.report-meta { margin-top: 0.25rem; font-family: var(--mono); font-size: 10px; color: var(--text3); letter-spacing: 0.04em; }
.report-context { margin-top: 0.4rem; font-size: 12.5px; line-height: 1.6; color: var(--text2); }

/* ── dup cluster in toc ──────────────────────────── */
.toc-cluster { border-bottom: 1px solid color-mix(in oklch, var(--border) 50%, transparent); }
.toc-cluster:last-child { border-bottom: none; }
.toc-cluster summary {
  display: flex; align-items: flex-start; gap: 12px;
  padding: 7px 0; cursor: pointer; list-style: none; user-select: none;
}
.toc-cluster summary::-webkit-details-marker { display: none; }
.toc-cluster summary:hover .toc-title-link { color: var(--text2); }
.toc-cluster-chevron { color: var(--text3); font-size: 10px; margin-top: 3px; flex-shrink: 0; width: 1rem; transition: transform .15s; }
details[open] .toc-cluster-chevron { transform: rotate(90deg); }
.toc-cluster-members { list-style: none; padding-left: 32px; }
.toc-cluster-member {
  display: flex; gap: 8px; align-items: flex-start;
  padding: 4px 0; border-bottom: 1px solid color-mix(in oklch, var(--border) 40%, transparent);
}
.toc-cluster-member:last-child { border-bottom: none; }

/* ── footer ──────────────────────────────────────── */
#footer {
  border-top: 1px solid var(--border);
  padding: 16px 40px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.footer-left, .footer-right {
  font-family: var(--mono);
  font-size: 10px;
  color: var(--text3);
  letter-spacing: 0.06em;
}

/* ── back to top ─────────────────────────────────── */
#back-top {
  position: fixed;
  bottom: 1.5rem;
  right: 1.5rem;
  background: var(--surface);
  border: 1px solid var(--border2);
  color: var(--text3);
  padding: 0.4rem 0.75rem;
  border-radius: var(--radius);
  font-family: var(--mono);
  font-size: 11px;
  letter-spacing: 0.06em;
  transition: color .15s, border-color .15s;
}
#back-top:hover { color: var(--cyan); border-color: var(--cyan); text-decoration: none; }

/* ── responsive ──────────────────────────────────── */
@media (max-width: 700px) {
  #nav { padding: 0 16px; }
  .nav-center { display: none; }
  #toc, #overview { padding-left: 16px; padding-right: 16px; }
  #footer { padding: 12px 16px; flex-direction: column; gap: 4px; align-items: flex-start; }
  .overview-grid { grid-template-columns: 1fr; }
  .overview-cell-border-r { border-right: none; border-top: 1px solid var(--border); }
  .overview-cell-full { grid-column: auto; }
  #back-top { bottom: 1rem; right: 0.75rem; }
}
</style>
</head>
<body>

<!-- Nav -->
<nav id="nav">
  <div class="nav-logo">
    <span class="nav-logo-down">DOWN</span><span class="nav-logo-link">LINK</span>
  </div>
  <div class="nav-center">
    <span class="nav-date">{{.StartedAt}}</span>
  </div>
  <div class="nav-right">
    <select id="digest-switcher" class="nav-switcher" aria-label="Switch provider/model"
            data-digest-id="{{.DigestId}}" data-article-set-hash="{{.ArticleSetHash}}" hidden></select>
    <span class="nav-window">{{.TimeWindow}} window</span>
    <div class="nav-count">
      <span class="nav-count-dot">■</span>
      <span>{{.ArticleCount}} articles</span>
      {{if .ModelName}}<span style="color:var(--text3)">·</span><span>{{.ModelName}}</span>{{end}}
    </div>
  </div>
</nav>

<!-- Reading list / TOC -->
<section id="toc">
  <div class="toc-card">
    <div class="toc-header" style="display:flex;align-items:center;justify-content:space-between">
      <span class="section-label">READING LIST</span>
      <div class="filter-bar">
        <button class="filter-btn active-all" data-filter="all" onclick="setFilter(this)">ALL <span class="filter-count">{{.ArticleCount}}</span></button>
        <button class="filter-btn" data-filter="must" onclick="setFilter(this)">MUST <span class="filter-count must-count"></span></button>
        <button class="filter-btn" data-filter="should" onclick="setFilter(this)">SHOULD <span class="filter-count should-count"></span></button>
        <button class="filter-btn" data-filter="may" onclick="setFilter(this)">MAY <span class="filter-count may-count"></span></button>
      </div>
    </div>
    <div class="toc-body">
      {{range .TOCGroups}}<div class="toc-group">
        <div class="toc-group-head">
          <span class="toc-group-label {{tocBadgeClass .Label}}">{{.Label}}</span>
          <span class="toc-group-count">{{len .Rows}} items</span>
        </div>
        {{range $i, $row := .Rows}}{{if $row.IsCluster}}<div class="toc-row-wrap {{priorityRowClass $row.Canonical.ReadTag}}" data-priority="{{priorityKey $row.Canonical.ReadTag}}">
          <details class="toc-cluster" style="border:none">
            <summary class="toc-row-inner" onclick="handleClusterClick(event,this)">
              <span class="toc-cluster-chevron">▶</span>
              <span class="toc-num {{tocNumClass $row.Canonical.ReadTag}}">{{printf "%02d" (add $i 1)}}</span>
              <div style="flex:1;min-width:0">
                <div style="display:flex;align-items:baseline;gap:7px;flex-wrap:wrap">
                  <span class="toc-title-link">{{$row.Canonical.Title}}</span>
                  <span class="group-badge" style="{{dupBadgeStyle $row.Group}}"></span>
                </div>
                {{if $row.CanonDetail}}<div class="toc-row-meta">
                  <span class="source-dot" style="background:{{sourceColorVal $row.CanonDetail.Source}}"></span>
                  <span class="source-name" style="color:{{sourceColorVal $row.CanonDetail.Source}}">{{$row.CanonDetail.Source}}</span>
                  <span class="meta-sep">·</span>
                  <span class="meta-time">{{$row.CanonDetail.PublishedAt}}</span>
                </div>{{end}}
              </div>
              {{if gt $row.Canonical.ImportanceScore 0}}{{scoreBar $row.Canonical.ImportanceScore}}{{end}}
              <span class="toc-row-chevron" id="chevron-{{$row.Canonical.Id}}">⌄</span>
            </summary>
            <ul class="toc-cluster-members">
              {{range $oi, $other := $row.Others}}<li class="toc-cluster-member">
                <span class="toc-child-indent">└</span>
                <div style="flex:1;min-width:0">
                  <div class="toc-child-title" onclick="toggleTocMember(this,'member-body-{{$other.Id}}')" style="cursor:pointer">{{$other.Title}}</div>
                  {{if index $row.OtherDetails $oi}}<div class="article-body" id="member-body-{{$other.Id}}">
                    {{$od := index $row.OtherDetails $oi}}
                    {{if $od.HasAnalysis}}<div class="analysis-strip">
                      <span><strong>Provider</strong> {{$od.Analysis.ProviderType}} / {{$od.Analysis.ModelName}}</span>
                      {{if gt $od.ImportanceScore 0}}<span><strong>Score</strong> {{$od.ImportanceScore}}/100</span>{{end}}
                      {{if $od.Analysis.Justification}}<span><strong>Why</strong> <span class="justification">{{$od.Analysis.Justification}}</span></span>{{end}}
                    </div>{{end}}
                    <div class="tabs">
                      {{if $od.HasAnalysis}}{{if $od.Analysis.Tldr}}<button class="tab-btn active" onclick="switchTab(this,'tldr-{{$other.Id}}')">TL;DR</button>
                      <button class="tab-btn" onclick="switchTab(this,'brief-{{$other.Id}}')">Brief</button>{{else}}<button class="tab-btn active" onclick="switchTab(this,'brief-{{$other.Id}}')">Brief</button>{{end}}
                      <button class="tab-btn" onclick="switchTab(this,'standard-{{$other.Id}}')">Standard</button>
                      <button class="tab-btn" onclick="switchTab(this,'comprehensive-{{$other.Id}}')">Full</button>
                      {{if $od.Analysis.KeyPoints}}<button class="tab-btn" onclick="switchTab(this,'keypoints-{{$other.Id}}')">Key Points</button>{{end}}
                      {{if $od.Analysis.Insights}}<button class="tab-btn" onclick="switchTab(this,'insights-{{$other.Id}}')">Insights</button>{{end}}
                      {{if $od.Analysis.ReferencedReports}}<button class="tab-btn" onclick="switchTab(this,'reports-{{$other.Id}}')">Reports</button>{{end}}
                      {{end}}
                    </div>
                    {{if $od.HasAnalysis}}
                    {{if $od.Analysis.Tldr}}<div id="tldr-{{$other.Id}}" class="tab-panel active"><div class="prose">{{$od.Analysis.Tldr}}</div></div>{{end}}
                    <div id="brief-{{$other.Id}}" class="tab-panel{{if not $od.Analysis.Tldr}} active{{end}}"><div class="prose">{{$od.Analysis.BriefOverview}}</div></div>
                    <div id="standard-{{$other.Id}}" class="tab-panel"><div class="prose">{{$od.Analysis.StandardSynthesis}}</div></div>
                    <div id="comprehensive-{{$other.Id}}" class="tab-panel"><div class="prose">{{$od.Analysis.ComprehensiveSynthesis}}</div></div>
                    {{if $od.Analysis.KeyPoints}}<div id="keypoints-{{$other.Id}}" class="tab-panel"><ul class="kp-list">{{range $od.Analysis.KeyPoints}}<li>{{.}}</li>{{end}}</ul></div>{{end}}
                    {{if $od.Analysis.Insights}}<div id="insights-{{$other.Id}}" class="tab-panel"><ul class="kp-list">{{range $od.Analysis.Insights}}<li>{{.}}</li>{{end}}</ul></div>{{end}}
                    {{if $od.Analysis.ReferencedReports}}<div id="reports-{{$other.Id}}" class="tab-panel"><ul class="report-list">{{range $od.Analysis.ReferencedReports}}<li class="report-item">
                      <div class="report-title"><a href="{{.URL}}" target="_blank" rel="noopener">{{if .Title}}{{.Title}}{{else}}{{.URL}}{{end}}</a></div>
                      {{if .Publisher}}<div class="report-meta">{{.Publisher}}</div>{{end}}
                      {{if .Context}}<div class="report-context">{{.Context}}</div>{{end}}
                    </li>{{end}}</ul></div>{{end}}
                    {{else}}<div class="tab-panel active" style="padding:1.25rem 1.4rem">
                      <a href="{{$od.Link}}" target="_blank" rel="noopener" style="font-family:var(--mono);font-size:10px;color:var(--cyan);letter-spacing:0.06em;padding:4px 10px;border:1px solid color-mix(in oklch, var(--cyan) 30%, transparent);border-radius:2px">OPEN SOURCE ↗</a>
                    </div>{{end}}
                  </div>{{end}}
                </div>
                {{if gt $other.ImportanceScore 0}}<span style="font-family:var(--mono);font-size:10px;color:var(--text3);flex-shrink:0">{{$other.ImportanceScore}}</span>{{end}}
              </li>{{end}}
            </ul>
          </details>
          {{if $row.CanonDetail}}<div class="toc-row-body" id="canon-body-{{$row.Canonical.Id}}">
            {{if $row.CanonDetail.HasAnalysis}}<div class="analysis-strip">
              <span><strong>Provider</strong> {{$row.CanonDetail.Analysis.ProviderType}} / {{$row.CanonDetail.Analysis.ModelName}}</span>
              {{if gt $row.CanonDetail.ImportanceScore 0}}<span><strong>Score</strong> {{$row.CanonDetail.ImportanceScore}}/100</span>{{end}}
              {{if $row.CanonDetail.Analysis.Justification}}<span><strong>Why</strong> <span class="justification">{{$row.CanonDetail.Analysis.Justification}}</span></span>{{end}}
            </div>{{end}}
            <div class="tabs">
              {{if $row.CanonDetail.HasAnalysis}}{{if $row.CanonDetail.Analysis.Tldr}}<button class="tab-btn active" onclick="switchTab(this,'tldr-{{$row.Canonical.Id}}')">TL;DR</button>
              <button class="tab-btn" onclick="switchTab(this,'brief-{{$row.Canonical.Id}}')">Brief</button>{{else}}<button class="tab-btn active" onclick="switchTab(this,'brief-{{$row.Canonical.Id}}')">Brief</button>{{end}}
              <button class="tab-btn" onclick="switchTab(this,'standard-{{$row.Canonical.Id}}')">Standard</button>
              <button class="tab-btn" onclick="switchTab(this,'comprehensive-{{$row.Canonical.Id}}')">Full</button>
              {{if $row.CanonDetail.Analysis.KeyPoints}}<button class="tab-btn" onclick="switchTab(this,'keypoints-{{$row.Canonical.Id}}')">Key Points</button>{{end}}
              {{if $row.CanonDetail.Analysis.Insights}}<button class="tab-btn" onclick="switchTab(this,'insights-{{$row.Canonical.Id}}')">Insights</button>{{end}}
              {{if $row.CanonDetail.Analysis.ReferencedReports}}<button class="tab-btn" onclick="switchTab(this,'reports-{{$row.Canonical.Id}}')">Reports</button>{{end}}
              {{end}}
            </div>
            {{if $row.CanonDetail.HasAnalysis}}
            {{if $row.CanonDetail.Analysis.Tldr}}<div id="tldr-{{$row.Canonical.Id}}" class="tab-panel active"><div class="prose">{{$row.CanonDetail.Analysis.Tldr}}</div></div>{{end}}
            <div id="brief-{{$row.Canonical.Id}}" class="tab-panel{{if not $row.CanonDetail.Analysis.Tldr}} active{{end}}"><div class="prose">{{$row.CanonDetail.Analysis.BriefOverview}}</div></div>
            <div id="standard-{{$row.Canonical.Id}}" class="tab-panel"><div class="prose">{{$row.CanonDetail.Analysis.StandardSynthesis}}</div></div>
            <div id="comprehensive-{{$row.Canonical.Id}}" class="tab-panel"><div class="prose">{{$row.CanonDetail.Analysis.ComprehensiveSynthesis}}</div></div>
            {{if $row.CanonDetail.Analysis.KeyPoints}}<div id="keypoints-{{$row.Canonical.Id}}" class="tab-panel"><ul class="kp-list">{{range $row.CanonDetail.Analysis.KeyPoints}}<li>{{.}}</li>{{end}}</ul></div>{{end}}
            {{if $row.CanonDetail.Analysis.Insights}}<div id="insights-{{$row.Canonical.Id}}" class="tab-panel"><ul class="kp-list">{{range $row.CanonDetail.Analysis.Insights}}<li>{{.}}</li>{{end}}</ul></div>{{end}}
            {{if $row.CanonDetail.Analysis.ReferencedReports}}<div id="reports-{{$row.Canonical.Id}}" class="tab-panel"><ul class="report-list">{{range $row.CanonDetail.Analysis.ReferencedReports}}<li class="report-item">
              <div class="report-title"><a href="{{.URL}}" target="_blank" rel="noopener">{{if .Title}}{{.Title}}{{else}}{{.URL}}{{end}}</a></div>
              {{if .Publisher}}<div class="report-meta">{{.Publisher}}</div>{{end}}
              {{if .Context}}<div class="report-context">{{.Context}}</div>{{end}}
            </li>{{end}}</ul></div>{{end}}
            {{else}}<div class="tab-panel active" style="padding:1.25rem 1.4rem">
              <a href="{{$row.CanonDetail.Link}}" target="_blank" rel="noopener" style="font-family:var(--mono);font-size:10px;color:var(--cyan);letter-spacing:0.06em;padding:4px 10px;border:1px solid color-mix(in oklch, var(--cyan) 30%, transparent);border-radius:2px">OPEN SOURCE ↗</a>
            </div>{{end}}
          </div>{{end}}
        </div>{{else}}<div class="toc-row-wrap {{priorityRowClass $row.Entry.ReadTag}}" data-priority="{{priorityKey $row.Entry.ReadTag}}">
          <div class="toc-row-inner" onclick="toggleTocRow(this)">
            <span class="toc-num {{tocNumClass $row.Entry.ReadTag}}">{{printf "%02d" (add $i 1)}}</span>
            <div style="flex:1;min-width:0">
              <div style="display:flex;align-items:baseline;gap:7px;flex-wrap:wrap">
                <span class="toc-title-link">{{$row.Entry.Title}}</span>
              </div>
              {{if $row.Detail}}<div class="toc-row-meta">
                <span class="source-dot" style="background:{{sourceColorVal $row.Detail.Source}}"></span>
                <span class="source-name" style="color:{{sourceColorVal $row.Detail.Source}}">{{$row.Detail.Source}}</span>
                <span class="meta-sep">·</span>
                <span class="meta-time">{{$row.Detail.PublishedAt}}</span>
              </div>{{end}}
            </div>
            {{if gt $row.Entry.ImportanceScore 0}}{{scoreBar $row.Entry.ImportanceScore}}{{end}}
            <span class="toc-row-chevron">⌄</span>
          </div>
          {{if $row.Detail}}<div class="toc-row-body">
            {{if $row.Detail.HasAnalysis}}<div class="analysis-strip">
              <span><strong>Provider</strong> {{$row.Detail.Analysis.ProviderType}} / {{$row.Detail.Analysis.ModelName}}</span>
              {{if gt $row.Detail.ImportanceScore 0}}<span><strong>Score</strong> {{$row.Detail.ImportanceScore}}/100</span>{{end}}
              {{if $row.Detail.Analysis.Justification}}<span><strong>Why</strong> <span class="justification">{{$row.Detail.Analysis.Justification}}</span></span>{{end}}
            </div>{{end}}
            <div class="tabs">
              {{if $row.Detail.HasAnalysis}}{{if $row.Detail.Analysis.Tldr}}<button class="tab-btn active" onclick="switchTab(this,'tldr-{{$row.Entry.Id}}')">TL;DR</button>
              <button class="tab-btn" onclick="switchTab(this,'brief-{{$row.Entry.Id}}')">Brief</button>{{else}}<button class="tab-btn active" onclick="switchTab(this,'brief-{{$row.Entry.Id}}')">Brief</button>{{end}}
              <button class="tab-btn" onclick="switchTab(this,'standard-{{$row.Entry.Id}}')">Standard</button>
              <button class="tab-btn" onclick="switchTab(this,'comprehensive-{{$row.Entry.Id}}')">Full</button>
              {{if $row.Detail.Analysis.KeyPoints}}<button class="tab-btn" onclick="switchTab(this,'keypoints-{{$row.Entry.Id}}')">Key Points</button>{{end}}
              {{if $row.Detail.Analysis.Insights}}<button class="tab-btn" onclick="switchTab(this,'insights-{{$row.Entry.Id}}')">Insights</button>{{end}}
              {{if $row.Detail.Analysis.ReferencedReports}}<button class="tab-btn" onclick="switchTab(this,'reports-{{$row.Entry.Id}}')">Reports</button>{{end}}
              {{end}}
            </div>
            {{if $row.Detail.HasAnalysis}}
            {{if $row.Detail.Analysis.Tldr}}<div id="tldr-{{$row.Entry.Id}}" class="tab-panel active"><div class="prose">{{$row.Detail.Analysis.Tldr}}</div></div>{{end}}
            <div id="brief-{{$row.Entry.Id}}" class="tab-panel{{if not $row.Detail.Analysis.Tldr}} active{{end}}"><div class="prose">{{$row.Detail.Analysis.BriefOverview}}</div></div>
            <div id="standard-{{$row.Entry.Id}}" class="tab-panel"><div class="prose">{{$row.Detail.Analysis.StandardSynthesis}}</div></div>
            <div id="comprehensive-{{$row.Entry.Id}}" class="tab-panel"><div class="prose">{{$row.Detail.Analysis.ComprehensiveSynthesis}}</div></div>
            {{if $row.Detail.Analysis.KeyPoints}}<div id="keypoints-{{$row.Entry.Id}}" class="tab-panel"><ul class="kp-list">{{range $row.Detail.Analysis.KeyPoints}}<li>{{.}}</li>{{end}}</ul></div>{{end}}
            {{if $row.Detail.Analysis.Insights}}<div id="insights-{{$row.Entry.Id}}" class="tab-panel"><ul class="kp-list">{{range $row.Detail.Analysis.Insights}}<li>{{.}}</li>{{end}}</ul></div>{{end}}
            {{if $row.Detail.Analysis.ReferencedReports}}<div id="reports-{{$row.Entry.Id}}" class="tab-panel"><ul class="report-list">{{range $row.Detail.Analysis.ReferencedReports}}<li class="report-item">
              <div class="report-title"><a href="{{.URL}}" target="_blank" rel="noopener">{{if .Title}}{{.Title}}{{else}}{{.URL}}{{end}}</a></div>
              {{if .Publisher}}<div class="report-meta">{{.Publisher}}</div>{{end}}
              {{if .Context}}<div class="report-context">{{.Context}}</div>{{end}}
            </li>{{end}}</ul></div>{{end}}
            {{else}}<div class="tab-panel active" style="padding:1.25rem 1.4rem">
              <a href="{{$row.Detail.Link}}" target="_blank" rel="noopener" style="font-family:var(--mono);font-size:10px;color:var(--cyan);letter-spacing:0.06em;padding:4px 10px;border:1px solid color-mix(in oklch, var(--cyan) 30%, transparent);border-radius:2px">OPEN SOURCE ↗</a>
            </div>{{end}}
          </div>{{end}}
        </div>{{end}}{{end}}
      </div>{{end}}
    </div>
  </div>
</section>

<!-- Intelligence Brief / Overview -->
{{if .OverviewSections}}<section id="overview">
  <div class="overview-card">
    <button class="overview-toggle" onclick="toggleOverview(this)">
      <span class="section-label">INTELLIGENCE BRIEF</span>
      <span class="overview-chevron" style="transform:rotate(180deg)">⌄</span>
    </button>
    <div class="overview-grid" id="overview-grid">
      {{range $i, $s := .OverviewSections}}<div class="overview-cell{{if eq $i 0}} overview-cell-full{{else if evenIndex $i}} overview-cell-border-r{{end}}{{if sectionBorderB $i (len $.OverviewSections)}} overview-cell-border-b{{end}}">
        <div class="overview-cell-head">
          <span class="overview-tag {{if eq $i 0}}overview-tag-exec{{else}}overview-tag-num{{end}}">{{$s.Tag}}</span>
          <span class="overview-title">{{$s.Title}}</span>
        </div>
        <div class="overview-body prose">{{$s.Body}}</div>
      </div>{{end}}
    </div>
  </div>
</section>{{end}}

<a id="back-top" href="#nav">↑ TOP</a>

<footer id="footer">
  <span class="footer-left">DOWNLINK · {{.StartedAt}} · {{.TimeWindow}} intelligence window</span>
  <span class="footer-right">{{.ArticleCount}} articles</span>
</footer>

<script>
function toggleTocRow(inner) {
  var row = inner.closest('.toc-row-wrap');
  var body = row.querySelector('.toc-row-body');
  var chevron = row.querySelector('.toc-row-chevron');
  if (!body) return;
  var isOpen = body.classList.contains('open');
  body.classList.toggle('open', !isOpen);
  row.classList.toggle('open', !isOpen);
  if (chevron) chevron.style.transform = isOpen ? '' : 'rotate(180deg)';
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
  if (chevron) chevron.style.transform = isOpen ? '' : 'rotate(180deg)';
  // Also toggle the cluster members <details> together with the body.
  if (details) details.open = !isOpen;
}

// Toggle an individual cluster member's body.
function toggleTocMember(titleEl, bodyId) {
  var body = document.getElementById(bodyId);
  if (!body) return;
  body.classList.toggle('open');
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

function setFilter(btn) {
  var f = btn.dataset.filter;
  document.querySelectorAll('.filter-btn').forEach(function(b){
    b.className = 'filter-btn';
    if (b.dataset.filter === f) {
      if (f === 'all') b.classList.add('active-all');
      else if (f === 'must') b.classList.add('active-must');
      else if (f === 'should') b.classList.add('active-should');
      else if (f === 'may') b.classList.add('active-may');
    }
  });
  document.querySelectorAll('.toc-row-wrap[data-priority]').forEach(function(row){
    if (f === 'all' || row.dataset.priority === f) {
      row.style.display = '';
    } else {
      row.style.display = 'none';
    }
  });
}

// update filter counts on load
(function() {
  var counts = {must:0, should:0, may:0};
  document.querySelectorAll('.toc-row-wrap[data-priority]').forEach(function(r){
    var p = r.dataset.priority;
    if (counts[p] !== undefined) counts[p]++;
  });
  document.querySelector('.must-count').textContent   = counts.must;
  document.querySelector('.should-count').textContent = counts.should;
  document.querySelector('.may-count').textContent    = counts.may;
})();

// keyboard: 'e' expand all, 'c' collapse all
document.addEventListener('keydown', function(e) {
  if (e.ctrlKey || e.altKey || e.metaKey) return;
  var t = e.target;
  if (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA') return;
  if (e.key === 'e') {
    document.querySelectorAll('.toc-row-wrap:not(.open)').forEach(function(row){
      var body = row.querySelector('.toc-row-body');
      var chevron = row.querySelector('.toc-row-chevron');
      if (body) { body.classList.add('open'); row.classList.add('open'); if (chevron) chevron.style.transform = 'rotate(180deg)'; }
    });
  }
  if (e.key === 'c') {
    document.querySelectorAll('.toc-row-wrap.open').forEach(function(row){
      var body = row.querySelector('.toc-row-body');
      var chevron = row.querySelector('.toc-row-chevron');
      if (body) { body.classList.remove('open'); row.classList.remove('open'); if (chevron) chevron.style.transform = ''; }
    });
  }
});

// provider/model switcher: pulls sibling list from manifest.json at runtime,
// filtering by matching articleSetHash. Stays hidden when there are <2 siblings
// or the manifest is unreachable.
(function() {
  var sel = document.getElementById('digest-switcher');
  if (!sel) return;
  sel.addEventListener('change', function(e) {
    var v = e.target.value;
    if (v) window.location.assign(v);
  });
  var hash = sel.dataset.articleSetHash;
  var currentId = sel.dataset.digestId;
  if (!hash) return;
  fetch('manifest.json', { cache: 'no-cache' }).then(function(r) {
    if (!r.ok) throw new Error('manifest fetch ' + r.status);
    return r.json();
  }).then(function(m) {
    var entries = (m && m.digests ? m.digests : []).filter(function(e) {
      return e.articleSetHash && e.articleSetHash === hash;
    });
    if (entries.length < 2) return;
    entries.forEach(function(e) {
      var opt = document.createElement('option');
      opt.value = e.filename;
      opt.textContent = e.providerType + ' / ' + e.modelName + ' · ' + e.displayDate;
      if (e.id === currentId) opt.selected = true;
      sel.appendChild(opt);
    });
    sel.hidden = false;
  }).catch(function(){ /* leave hidden */ });
})();
</script>
</body>
</html>
`

// TOCEntry holds data for a single article row in the table of contents.
type TOCEntry struct {
	Id                  string
	Title               string
	ImportanceScore     int
	ReadTag             string
	DuplicateGroup      string
	IsMostComprehensive bool
}

// RenderedAnalysis holds markdown-converted HTML versions of analysis text fields.
type RenderedAnalysis struct {
	ProviderType           string
	ModelName              string
	Tldr                   string
	Justification          template.HTML
	BriefOverview          template.HTML
	StandardSynthesis      template.HTML
	ComprehensiveSynthesis template.HTML
	KeyPoints              []string
	Insights               []string
	ReferencedReports      []models.ReferencedReport
}

// ArticleEntry holds enriched article data for template rendering
type ArticleEntry struct {
	Id                  string
	Title               string
	Source              string
	Link                string
	PublishedAt         string
	ImportanceScore     int
	ReadTag             string
	DuplicateGroup      string
	IsMostComprehensive bool
	HasAnalysis         bool
	Analysis            *RenderedAnalysis
}

// readTag returns a priority label based on a 1-100 importance score, matching the UI thresholds.
func readTag(score int) string {
	switch {
	case score >= 90:
		return "Must Read"
	case score >= 75:
		return "Should Read"
	case score >= 60:
		return "May Read"
	case score > 0:
		return "Optional"
	default:
		return "Unscored"
	}
}

// tagOrder defines the display order of read-tag groups in the TOC.
var tagOrder = []string{"Must Read", "Should Read", "May Read", "Optional", "Unscored"}

// TOCRow is a single rendered row in a TOC group: either a plain article entry or
// a duplicate cluster (one canonical article + its alternates in a <details> block).
type TOCRow struct {
	IsCluster    bool
	Entry        TOCEntry        // used when IsCluster == false
	Canonical    TOCEntry        // used when IsCluster == true: the most-comprehensive article
	Others       []TOCEntry      // used when IsCluster == true: remaining members
	Group        string          // duplicate group key, used for colour
	Detail       *ArticleEntry   // full detail for non-cluster row
	CanonDetail  *ArticleEntry   // full detail for cluster canonical
	OtherDetails []*ArticleEntry // full detail for each cluster member
}

// TOCGroup is a labelled section in the table of contents.
type TOCGroup struct {
	Label string
	Rows  []TOCRow
}

// buildTOCGroups partitions already-sorted TOC entries into labelled groups, collapsing
// duplicate clusters into a single <details> row placed in the highest-priority group
// any cluster member appears in.
func buildTOCGroups(entries []TOCEntry) []TOCGroup {
	// Phase 1: collect per-group members and track highest-priority tag for each cluster.
	tagPriority := make(map[string]int, len(tagOrder))
	for i, t := range tagOrder {
		tagPriority[t] = i
	}

	type clusterInfo struct {
		canonical TOCEntry
		others    []TOCEntry
		bestTag   string
		bestPrio  int
	}
	clusters := make(map[string]*clusterInfo)
	var clusterOrder []string // insertion order for stable output

	var plain []TOCEntry // non-duplicate entries

	for _, e := range entries {
		if e.DuplicateGroup == "" {
			plain = append(plain, e)
			continue
		}
		ci, exists := clusters[e.DuplicateGroup]
		if !exists {
			ci = &clusterInfo{bestPrio: len(tagOrder)}
			clusters[e.DuplicateGroup] = ci
			clusterOrder = append(clusterOrder, e.DuplicateGroup)
		}
		prio := tagPriority[e.ReadTag]
		if prio < ci.bestPrio {
			ci.bestPrio = prio
			ci.bestTag = e.ReadTag
		}
		if e.IsMostComprehensive {
			ci.canonical = e
		} else {
			ci.others = append(ci.others, e)
		}
	}

	// Phase 2: build priority buckets of TOCRows.
	type bucket struct {
		rows []TOCRow
	}
	buckets := make(map[string]*bucket, len(tagOrder))
	for _, t := range tagOrder {
		buckets[t] = &bucket{}
	}

	// Place each cluster row into the bucket of its highest-priority tag.
	for _, g := range clusterOrder {
		ci := clusters[g]
		// If no member was marked most-comprehensive, promote the first other.
		if ci.canonical.Id == "" && len(ci.others) > 0 {
			ci.canonical = ci.others[0]
			ci.others = ci.others[1:]
		}
		buckets[ci.bestTag].rows = append(buckets[ci.bestTag].rows, TOCRow{
			IsCluster: true,
			Canonical: ci.canonical,
			Others:    ci.others,
			Group:     g,
		})
	}

	// Place plain entries into their respective buckets.
	for _, e := range plain {
		b := buckets[e.ReadTag]
		b.rows = append(b.rows, TOCRow{Entry: e})
	}

	// Phase 3: sort each bucket: clusters by canonical score, plain by score (already sorted
	// globally, but plain entries need stable interleaving with cluster rows).
	// Simple approach: re-sort each bucket by lead score descending.
	for _, b := range buckets {
		sort.SliceStable(b.rows, func(i, j int) bool {
			si := b.rows[i].leadScore()
			sj := b.rows[j].leadScore()
			return si > sj
		})
	}

	var groups []TOCGroup
	for _, label := range tagOrder {
		if b := buckets[label]; len(b.rows) > 0 {
			groups = append(groups, TOCGroup{Label: label, Rows: b.rows})
		}
	}
	return groups
}

func (r TOCRow) leadScore() int {
	if r.IsCluster {
		return r.Canonical.ImportanceScore
	}
	return r.Entry.ImportanceScore
}

// OverviewSection is a single cell in the Intelligence Brief 2-column grid.
type OverviewSection struct {
	Tag   string        // e.g. "EXEC", "01", "02"
	Title string        // section heading
	Body  template.HTML // rendered markdown body
}

// digestTemplateData is the root data passed to the HTML template
type digestTemplateData struct {
	StartedAt        string
	ArticleCount     int
	ModelName        string
	TimeWindow       string
	ThemeOverride    template.CSS
	DigestSummary    template.HTML // kept for backwards compat; OverviewSections is used for rendering
	OverviewSections []OverviewSection
	TOCGroups        []TOCGroup
	ArticleEntries   []ArticleEntry
	DigestId         string
	ArticleSetHash   string
}

// RenderDigestHTML generates a self-contained HTML file for the given digest.
// The digest must have Articles, DigestAnalyses (with Analysis preloaded), and ProviderResults populated.
// theme selects the visual style; an empty string or "dark" uses the default dark theme.
//
// The provider/model switcher in the rendered page is populated client-side
// from manifest.json — the page itself only embeds the digest id and a hash
// of its article set used to filter siblings.
func RenderDigestHTML(digest models.Digest, theme string) ([]byte, error) {
	// Build a lookup: articleId → DigestAnalysis (for duplicate metadata and analysis)
	daByArticle := make(map[string]models.DigestAnalysis, len(digest.DigestAnalyses))
	for _, da := range digest.DigestAnalyses {
		daByArticle[da.ArticleId] = da
	}

	var tocEntries []TOCEntry
	var articleEntries []ArticleEntry
	var digestModelName string

	for _, art := range digest.Articles {
		da := daByArticle[art.Id]

		var analysis *models.ArticleAnalysis
		var importanceScore int
		if da.Analysis != nil {
			analysis = da.Analysis
			importanceScore = da.Analysis.ImportanceScore
		}

		tag := readTag(importanceScore)

		tocEntries = append(tocEntries, TOCEntry{
			Id:                  art.Id,
			Title:               articleTitle(art.Title),
			ImportanceScore:     importanceScore,
			ReadTag:             tag,
			DuplicateGroup:      da.DuplicateGroup,
			IsMostComprehensive: da.IsMostComprehensive,
		})

		var rendered *RenderedAnalysis
		if analysis != nil {
			if digestModelName == "" {
				digestModelName = analysis.ModelName
			}
			rendered = &RenderedAnalysis{
				ProviderType:           analysis.ProviderType,
				ModelName:              analysis.ModelName,
				Tldr:                   analysis.Tldr,
				Justification:          markdownToHTML(analysis.Justification),
				BriefOverview:          markdownToHTML(analysis.BriefOverview),
				StandardSynthesis:      markdownToHTML(analysis.StandardSynthesis),
				ComprehensiveSynthesis: markdownToHTML(analysis.ComprehensiveSynthesis),
				KeyPoints:              analysis.KeyPoints,
				Insights:               analysis.Insights,
				ReferencedReports:      analysis.ReferencedReports,
			}
		}

		articleEntries = append(articleEntries, ArticleEntry{
			Id:                  art.Id,
			Title:               articleTitle(art.Title),
			Source:              articleSource(art.Link),
			Link:                art.Link,
			PublishedAt:         art.PublishedAt.Format("2006-01-02 15:04"),
			ImportanceScore:     importanceScore,
			ReadTag:             tag,
			DuplicateGroup:      da.DuplicateGroup,
			IsMostComprehensive: da.IsMostComprehensive,
			HasAnalysis:         rendered != nil,
			Analysis:            rendered,
		})
	}

	articleCount := len(articleEntries)

	// Sort TOC by importance score descending before grouping.
	sort.Slice(tocEntries, func(i, j int) bool {
		return tocEntries[i].ImportanceScore > tocEntries[j].ImportanceScore
	})

	tocGroups := buildTOCGroups(tocEntries)

	// Build id→detail map and attach full article data to each TOC row.
	detailByID := make(map[string]*ArticleEntry, len(articleEntries))
	for i := range articleEntries {
		detailByID[articleEntries[i].Id] = &articleEntries[i]
	}
	for gi := range tocGroups {
		for ri := range tocGroups[gi].Rows {
			row := &tocGroups[gi].Rows[ri]
			if row.IsCluster {
				row.CanonDetail = detailByID[row.Canonical.Id]
				row.OtherDetails = make([]*ArticleEntry, len(row.Others))
				for oi, o := range row.Others {
					row.OtherDetails[oi] = detailByID[o.Id]
				}
			} else {
				row.Detail = detailByID[row.Entry.Id]
			}
		}
	}

	var themeOverride template.CSS
	if t, ok := digestthemes.Get(theme); ok && t.Vars != nil {
		var sb strings.Builder
		for k, v := range t.Vars {
			sb.WriteString(k)
			sb.WriteString(": ")
			sb.WriteString(v)
			sb.WriteString("; ")
		}
		themeOverride = template.CSS(sb.String()) //nolint:gosec // values come from our own hardcoded theme map
	}

	data := digestTemplateData{
		StartedAt:        digest.CreatedAt.Add(-digest.TimeWindow).Format("2006-01-02 15:04 UTC"),
		ArticleCount:     articleCount,
		ModelName:        digestModelName,
		TimeWindow:       formatDuration(digest.TimeWindow),
		DigestSummary:    markdownToHTML(digest.DigestSummary),
		OverviewSections: parseOverviewSections(digest.DigestSummary),
		TOCGroups:        tocGroups,
		ArticleEntries:   articleEntries,
		ThemeOverride:    themeOverride,
		DigestId:         digest.Id,
		ArticleSetHash:   ArticleSetHash(digest),
	}

	funcMap := template.FuncMap{
		"add":                func(a, b int) int { return a + b },
		"slugify":            func(s string) string { return strings.ReplaceAll(s, " ", "-") },
		"dupColor":           dupGroupColor,
		"sourceColor":        sourceColor,
		"sourceColorVal":     sourceColorVal,
		"dupBadgeStyle":      dupBadgeStyle,
		"dupGroupLetter":     dupGroupLetter,
		"tocBadgeClass":      tocBadgeClass,
		"tocNumClass":        tocNumClass,
		"priorityRowClass":   priorityRowClass,
		"priorityBadgeClass": priorityBadgeClass,
		"priorityShort":      priorityShort,
		"priorityKey":        priorityKey,
		"scoreBar":           scoreBarHTML,
		// overview grid helpers
		// evenIndex: i=0 is EXEC (full-width); i=2,4,6… are left-column cells → get border-right
		"evenIndex": func(i int) bool { return i%2 == 0 },
		// sectionBorderB: border-bottom on all cells except the last two (the last pair)
		"sectionBorderB": func(i, total int) bool { return i < total-2 },
	}

	tmpl, err := template.New("digest").Funcs(funcMap).Parse(digestHTMLTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse digest HTML template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to render digest HTML: %w", err)
	}

	return buf.Bytes(), nil
}

// DigestHTMLFilename returns a filesystem-safe filename for the digest HTML file.
func DigestHTMLFilename(digest models.Digest) string {
	ts := digest.CreatedAt.UTC().Format("2006-01-02_1504")
	return fmt.Sprintf("downlink-digest-%s.html", ts)
}

// parseOverviewSections splits a markdown digest summary into OverviewSection blocks.
// It splits on level-2 headings (## Heading). The content before the first heading
// (if any) becomes the EXEC section. Each subsequent ## heading produces a numbered
// section (01, 02, …). If there are no ## headings the entire text becomes one EXEC cell.
func parseOverviewSections(md string) []OverviewSection {
	if strings.TrimSpace(md) == "" {
		return nil
	}

	lines := strings.Split(md, "\n")
	type rawSection struct {
		title string
		lines []string
	}

	var sections []rawSection
	var cur *rawSection

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			if cur != nil {
				sections = append(sections, *cur)
			}
			cur = &rawSection{title: strings.TrimPrefix(line, "## ")}
		} else {
			if cur == nil {
				cur = &rawSection{title: "Executive Overview"}
			}
			cur.lines = append(cur.lines, line)
		}
	}
	if cur != nil {
		sections = append(sections, *cur)
	}

	if len(sections) == 0 {
		return nil
	}

	result := make([]OverviewSection, 0, len(sections))
	numbered := 0
	for i, s := range sections {
		body := strings.TrimSpace(strings.Join(s.lines, "\n"))
		var tag string
		if i == 0 && (s.title == "Executive Overview" || len(sections) == 1) {
			tag = "EXEC"
		} else {
			numbered++
			tag = fmt.Sprintf("%02d", numbered)
		}
		result = append(result, OverviewSection{
			Tag:   tag,
			Title: s.title,
			Body:  markdownToHTML(body),
		})
	}
	return result
}

// markdownToHTML converts a markdown string to sanitized HTML using gomarkdown.
func markdownToHTML(md string) template.HTML {
	if md == "" {
		return ""
	}
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	opts := mdhtml.RendererOptions{Flags: mdhtml.CommonFlags | mdhtml.HrefTargetBlank}
	renderer := mdhtml.NewRenderer(opts)
	output := markdown.ToHTML([]byte(md), p, renderer)
	return template.HTML(output) //nolint:gosec // markdown is LLM-generated content stored in our own DB
}

// colorPalette is a set of visually distinct colors used for source and duplicate group dots.
var colorPalette = []string{
	"#f87171", // red
	"#fb923c", // orange
	"#ca9a04", // yellow
	"#4ade80", // green
	"#2dd4bf", // teal
	"#60a5fa", // blue
	"#c084fc", // purple
	"#f472b6", // pink
	"#a78bfa", // violet
	"#34d399", // emerald
	"#38bdf8", // sky
	"#e879f9", // fuchsia
}

// paletteColor hashes a string to a consistent color from colorPalette.
func paletteColor(s string) string {
	var h uint32
	for _, c := range s {
		h = h*31 + uint32(c)
	}
	return colorPalette[h%uint32(len(colorPalette))]
}

// dupGroupColor returns an inline CSS background style for a duplicate group dot.
func dupGroupColor(group string) template.CSS {
	return template.CSS(fmt.Sprintf("background:%s", paletteColor(group)))
}

// sourceColor returns an inline CSS background style for a source dot.
func sourceColor(source string) template.CSS {
	return template.CSS(fmt.Sprintf("background:%s", paletteColor(source)))
}

// sourceColorVal returns just the color value string (no "background:" prefix).
func sourceColorVal(source string) string {
	return paletteColor(source)
}

// dupGroupLetter returns a short letter label for a duplicate group key.
func dupGroupLetter(group string) string {
	if group == "" {
		return ""
	}
	// Use the first character of the hashed index to produce A, B, C… labels.
	var h uint32
	for _, c := range group {
		h = h*31 + uint32(c)
	}
	return string(rune('A' + h%26))
}

// dupBadgeStyle returns inline CSS for a group badge (color + border + background).
func dupBadgeStyle(group string) template.CSS {
	c := paletteColor(group)
	return template.CSS(fmt.Sprintf("color:%s;border:1px solid %s40;background:%s1a", c, c, c))
}

// tocBadgeClass returns the CSS class for a TOC group header priority badge.
func tocBadgeClass(label string) string {
	switch label {
	case "Must Read":
		return "priority-badge badge-must"
	case "Should Read":
		return "priority-badge badge-should"
	case "May Read":
		return "priority-badge badge-may"
	default:
		return "priority-badge badge-opt"
	}
}

// tocNumClass returns the CSS class for a TOC row number based on read tag.
func tocNumClass(tag string) string {
	switch tag {
	case "Must Read":
		return "toc-num-must"
	case "Should Read":
		return "toc-num-should"
	case "May Read":
		return "toc-num-may"
	default:
		return "toc-num-opt"
	}
}

// priorityRowClass returns the CSS class for an article row's priority rail.
func priorityRowClass(tag string) string {
	switch tag {
	case "Must Read":
		return "must-row"
	case "Should Read":
		return "should-row"
	case "May Read":
		return "may-row"
	default:
		return ""
	}
}

// priorityBadgeClass returns the CSS class for an article priority badge.
func priorityBadgeClass(tag string) string {
	switch tag {
	case "Must Read":
		return "badge-must"
	case "Should Read":
		return "badge-should"
	case "May Read":
		return "badge-may"
	default:
		return "badge-opt"
	}
}

// priorityShort returns the short label used in the priority badge.
func priorityShort(tag string) string {
	switch tag {
	case "Must Read":
		return "MUST"
	case "Should Read":
		return "SHOULD"
	case "May Read":
		return "MAY"
	default:
		return tag
	}
}

// priorityKey returns the filter key used in data-priority attributes.
func priorityKey(tag string) string {
	switch tag {
	case "Must Read":
		return "must"
	case "Should Read":
		return "should"
	case "May Read":
		return "may"
	default:
		return "opt"
	}
}

// scoreBarHTML renders an inline score bar for the articles list.
func scoreBarHTML(score int) template.HTML {
	var fillClass, numClass string
	switch {
	case score >= 90:
		fillClass, numClass = "score-fill score-fill-high", "score-num score-num-high"
	case score >= 75:
		fillClass, numClass = "score-fill score-fill-mid", "score-num score-num-mid"
	default:
		fillClass, numClass = "score-fill score-fill-low", "score-num score-num-low"
	}
	return template.HTML(fmt.Sprintf( //nolint:gosec
		`<div class="score-bar"><div class="score-track"><div class="%s" style="width:%d%%"></div></div><span class="%s">%d</span></div>`,
		fillClass, score, numClass, score,
	))
}

func articleTitle(t string) string {
	if t == "" {
		return "Untitled"
	}
	return t
}

const digestIndexTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Downlink Digests</title>
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --bg: #111214; --surface: #18191d; --border: #25272c;
  --text: #e2e4e9; --muted: #7c8091; --accent: #5865F2;
}
body { background: var(--bg); color: var(--text); font-family: system-ui, sans-serif; padding: 2rem; }
h1 { font-size: 1.5rem; margin-bottom: 1.5rem; color: var(--accent); }
ul { list-style: none; display: flex; flex-direction: column; gap: .5rem; }
li a { color: var(--text); text-decoration: none; padding: .6rem 1rem; display: block;
       background: var(--surface); border: 1px solid var(--border); border-radius: 6px; }
li a:hover { border-color: var(--accent); color: var(--accent); }
li .meta { color: var(--muted); font-size: .85em; margin-left: .5rem; }
#empty { color: var(--muted); }
</style>
</head>
<body>
<h1>📰 Downlink Digests</h1>
<ul id="digest-list" data-manifest-url="manifest.json"><li id="empty">Loading…</li></ul>
<script>
(function() {
  var list = document.getElementById('digest-list');
  var manifestURL = list.getAttribute('data-manifest-url') || 'manifest.json';
  fetch(manifestURL, { cache: 'no-cache' }).then(function(r) {
    if (!r.ok) throw new Error('manifest fetch ' + r.status);
    return r.json();
  }).then(function(m) {
    var entries = (m && m.digests) ? m.digests : [];
    list.innerHTML = '';
    if (!entries.length) {
      list.innerHTML = '<li id="empty">No digests yet.</li>';
      return;
    }
    entries.forEach(function(e) {
      var li = document.createElement('li');
      var a = document.createElement('a');
      a.href = e.filename;
      a.textContent = e.displayDate;
      var meta = document.createElement('span');
      meta.className = 'meta';
      var label = [e.providerType, e.modelName].filter(Boolean).join(' / ');
      if (label) meta.textContent = ' — ' + label;
      a.appendChild(meta);
      li.appendChild(a);
      list.appendChild(li);
    });
  }).catch(function(err) {
    list.innerHTML = '<li id="empty">Failed to load digest list.</li>';
  });
})();
</script>
</body>
</html>`

// RenderDigestIndex generates the index HTML shell. The digest list is
// populated client-side by fetching manifest.json, so the rendered bytes are
// constant for a given template.
func RenderDigestIndex() ([]byte, error) {
	tmpl, err := template.New("index").Parse(digestIndexTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse index template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, nil); err != nil {
		return nil, fmt.Errorf("failed to render digest index: %w", err)
	}
	return buf.Bytes(), nil
}
