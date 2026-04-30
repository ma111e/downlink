package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"text/template"

	"downlink/pkg/models"
)

// swipeArticle is the JSON representation of an article for the swipe triage view.
type swipeArticle struct {
	N           int      `json:"n"`
	Title       string   `json:"title"`
	Source      string   `json:"source"`
	SourceColor string   `json:"sourceColor"`
	Link        string   `json:"link"`
	Time        string   `json:"time"`
	Priority    string   `json:"priority"`
	Score       int      `json:"score"`
	Group       *string  `json:"group"`
	GroupColor  *string  `json:"groupColor"`
	Tldr        string   `json:"tldr"`
	KeyPoints   []string `json:"keyPoints"`
	Body        string   `json:"body"`
}

type swipeTemplateData struct {
	DigestFilename string
	TimeWindow     string
	ArticlesJSON   string
}

var swipePriorityRank = map[string]int{
	"MUST READ":   0,
	"SHOULD READ": 1,
	"MAY READ":    2,
}

func swipePriorityLabel(tag string) string {
	switch tag {
	case "Must Read":
		return "MUST READ"
	case "Should Read":
		return "SHOULD READ"
	default:
		return "MAY READ"
	}
}

// RenderSwipeHTML generates the self-contained Tinder-style triage page for a digest.
// digestFilename is the filename of the companion list-view page (used for the back link).
func RenderSwipeHTML(digest models.Digest, digestFilename string) ([]byte, error) {
	daByArticle := make(map[string]models.DigestAnalysis, len(digest.DigestAnalyses))
	for _, da := range digest.DigestAnalyses {
		daByArticle[da.ArticleId] = da
	}

	articles := make([]swipeArticle, 0, len(digest.Articles))
	for i, art := range digest.Articles {
		da := daByArticle[art.Id]

		var score int
		var tldr string
		var keyPoints []string
		var body string
		if da.Analysis != nil {
			score = da.Analysis.ImportanceScore
			tldr = da.Analysis.Tldr
			keyPoints = da.Analysis.KeyPoints
			src := da.Analysis.BriefOverview
			if src == "" {
				src = da.Analysis.StandardSynthesis
			}
			body = string(markdownToHTML(src))
		}
		if keyPoints == nil {
			keyPoints = []string{}
		}

		tag := readTag(score)
		srcDomain := articleSource(art.Link)

		var group, groupColor *string
		if da.DuplicateGroup != "" {
			g := dupGroupLetter(da.DuplicateGroup)
			c := paletteColor(da.DuplicateGroup)
			group = &g
			groupColor = &c
		}

		articles = append(articles, swipeArticle{
			N:           i + 1,
			Title:       articleTitle(art.Title),
			Source:      srcDomain,
			SourceColor: paletteColor(srcDomain),
			Link:        art.Link,
			Time:        art.PublishedAt.Format("15:04"),
			Priority:    swipePriorityLabel(tag),
			Score:       score,
			Group:       group,
			GroupColor:  groupColor,
			Tldr:        tldr,
			KeyPoints:   keyPoints,
			Body:        body,
		})
	}

	sort.SliceStable(articles, func(i, j int) bool {
		pi := swipePriorityRank[articles[i].Priority]
		pj := swipePriorityRank[articles[j].Priority]
		if pi != pj {
			return pi < pj
		}
		return articles[i].Score > articles[j].Score
	})

	articlesJSON, err := json.Marshal(articles)
	if err != nil {
		return nil, fmt.Errorf("swipe: marshal articles: %w", err)
	}

	data := swipeTemplateData{
		DigestFilename: digestFilename,
		TimeWindow:     formatDuration(digest.TimeWindow),
		ArticlesJSON:   string(articlesJSON),
	}

	// Use <% %> delimiters so JSX {{ }} syntax in the template is left untouched.
	tmpl, err := template.New("swipe").Delims("<%", "%>").Parse(swipeHTMLTemplate)
	if err != nil {
		return nil, fmt.Errorf("swipe: parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("swipe: render: %w", err)
	}
	return buf.Bytes(), nil
}

// SwipeHTMLFilename returns the filename for the swipe triage view of a digest.
func SwipeHTMLFilename(digest models.Digest) string {
	ts := digest.CreatedAt.UTC().Format("2006-01-02_1504")
	return fmt.Sprintf("downlink-swipe-%s.html", ts)
}

// swipeHTMLTemplate is the Tinder-style triage view template.
// Uses <% %> Go template delimiters to avoid conflicts with JSX {{ }} syntax.
// JS template literals (backticks) are replaced with string concatenation to
// avoid conflicting with the Go raw string delimiter.
var swipeHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>DOWNLINK · Triage</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500;600&family=IBM+Plex+Sans:wght@300;400;500;600&display=swap" rel="stylesheet">
<script src="https://unpkg.com/react@18.3.1/umd/react.development.js" integrity="sha384-hD6/rw4ppMLGNu3tX5cjIb+uRZ7UkRJ6BPkLpg4hAu/6onKUg4lLsHAs9EBPT82L" crossorigin="anonymous"></script>
<script src="https://unpkg.com/react-dom@18.3.1/umd/react-dom.development.js" integrity="sha384-u6aeetuaXnQ38mYT8rp6sbXaQe3NL9t+IBXmnYxwkUI2Hw4bsp2Wvmx4yRQF1uAm" crossorigin="anonymous"></script>
<script src="https://unpkg.com/@babel/standalone@7.29.0/babel.min.js" integrity="sha384-m08KidiNqLdpJqLq95G/LEi8Qvjl/xUYll3QILypMoQ65QorJ9Lvtp2RXYGBFj1y" crossorigin="anonymous"></script>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  :root {
    --bg: #0b0b0e;
    --surface: #111116;
    --surface2: #15151c;
    --border: #1c1c25;
    --border2: #252530;
    --text: #dddde8;
    --text2: #a8a8be;
    --text3: #4a4a60;
    --cyan: oklch(74% 0.14 196);
    --must:   oklch(66% 0.15 38);
    --should: oklch(73% 0.16 60);
    --may:    oklch(65% 0.13 250);
    --discard: oklch(62% 0.20 22);
    --accept:  oklch(70% 0.16 155);
  }
  html {
    background: var(--bg);
    background-image: radial-gradient(circle, #ffffff08 1px, transparent 1px);
    background-size: 24px 24px;
    color: var(--text);
    font-family: 'IBM Plex Sans', sans-serif;
    font-size: 14px; line-height: 1.6;
  }
  body { min-height: 100vh; overflow-x: hidden; }
  ::selection { background: color-mix(in oklch, var(--cyan) 25%, transparent); }
  ::-webkit-scrollbar { width: 6px; }
  ::-webkit-scrollbar-thumb { background: var(--border2); border-radius: 3px; }

  @keyframes pulse-ring {
    0%   { box-shadow: 0 0 0 0 color-mix(in oklch, var(--cyan) 60%, transparent); }
    70%  { box-shadow: 0 0 0 5px transparent; }
    100% { box-shadow: 0 0 0 0 transparent; }
  }
  .live-dot { width: 7px; height: 7px; border-radius: 50%; background: var(--cyan); animation: pulse-ring 2s ease-out infinite; }
  .section-label { font-family: 'IBM Plex Mono', monospace; font-size: 10px; letter-spacing: 0.14em; color: var(--text3); font-weight: 500; display: flex; align-items: center; gap: 8px; }
  .section-label::before { content: "//"; color: var(--text3); opacity: 0.5; font-size: 11px; }

  .card-stack { position: relative; width: 100%; max-width: 520px; height: 620px; margin: 0 auto; }
  .swipe-card {
    position: absolute; inset: 0;
    background: var(--surface); border: 1px solid var(--border); border-radius: 8px;
    overflow: hidden; display: flex; flex-direction: column;
    box-shadow: 0 18px 50px rgba(0,0,0,0.5);
    will-change: transform; user-select: none; -webkit-user-select: none;
  }
  .swipe-card.dragging { transition: none; cursor: grabbing; }
  .swipe-card.snap-back { transition: transform 0.32s cubic-bezier(.2,.9,.2,1); }
  .swipe-card.fly-out  { transition: transform 0.4s cubic-bezier(.4,.0,.6,1), opacity 0.4s; }

  .stamp {
    position: absolute; top: 28px;
    font-family: 'IBM Plex Mono', monospace; font-weight: 600; font-size: 26px; letter-spacing: 0.14em;
    padding: 8px 16px; border: 3px solid; border-radius: 6px;
    pointer-events: none; text-transform: uppercase; opacity: 0; transition: opacity 0.1s;
  }
  .stamp.skip { left: 28px; transform: rotate(-14deg); color: var(--discard); border-color: var(--discard); }
  .stamp.read { right: 28px; transform: rotate(14deg);  color: var(--accept);  border-color: var(--accept); }

  .pill-btn {
    all: unset; cursor: pointer;
    width: 56px; height: 56px; border-radius: 50%;
    display: flex; align-items: center; justify-content: center;
    border: 1px solid var(--border2); background: var(--surface); color: var(--text);
    transition: transform 0.12s, background 0.12s, border-color 0.12s, color 0.12s;
  }
  .pill-btn:hover { transform: scale(1.06); }
  .pill-btn.skip:hover { border-color: var(--discard); color: var(--discard); }
  .pill-btn.read:hover { border-color: var(--accept);  color: var(--accept); }

  .progress-bar { height: 3px; background: var(--border); border-radius: 2px; overflow: hidden; }
  .progress-bar > div { height: 100%; background: var(--cyan); transition: width 0.3s; }

  .detail-scroll { overflow-y: auto; }
  .detail-scroll::-webkit-scrollbar { width: 4px; }

  .prose { line-height: 1.8; color: var(--text2); font-size: 13px; }
  .prose p { margin-bottom: 0.75rem; }
  .prose p:last-child { margin-bottom: 0; }
  .prose ul, .prose ol { padding-left: 1.5rem; margin-bottom: 0.75rem; }
  .prose li { margin-bottom: 0.3rem; }
  .prose strong { color: var(--text); font-weight: 600; }
  .prose a { color: var(--cyan); }
  .prose h1, .prose h2, .prose h3 { color: var(--text); font-weight: 600; margin: 1rem 0 0.4rem; font-size: 0.9rem; }
</style>
</head>
<body>
<script>
window.__DL_ARTICLES = <% .ArticlesJSON %>;
window.__DL_DIGEST   = "<% .DigestFilename %>";
window.__DL_WINDOW   = "<% .TimeWindow %>";
</script>
<script type="text/babel">

const ARTICLES_RAW = window.__DL_ARTICLES;
const DIGEST_HREF  = window.__DL_DIGEST;
const TIME_WINDOW  = window.__DL_WINDOW;

const PRIORITY_ORDER = { "MUST READ": 0, "SHOULD READ": 1, "MAY READ": 2 };

const ARTICLES = [...ARTICLES_RAW].sort((a, b) => {
  const p = PRIORITY_ORDER[a.priority] - PRIORITY_ORDER[b.priority];
  return p !== 0 ? p : b.score - a.score;
});

function priorityColor(p) {
  if (p === "MUST READ")   return "var(--must)";
  if (p === "SHOULD READ") return "var(--should)";
  return "var(--may)";
}
function priorityShort(p) {
  return p === "MUST READ" ? "MUST" : p === "SHOULD READ" ? "SHOULD" : "MAY";
}

function PriorityBadge({ priority }) {
  const color = priorityColor(priority);
  return (
    <span style={{
      fontFamily: "'IBM Plex Mono', monospace", fontSize: 10, fontWeight: 600, letterSpacing: "0.08em", color,
      border: "1px solid color-mix(in oklch, " + color + " 45%, transparent)",
      background: "color-mix(in oklch, " + color + " 12%, transparent)",
      padding: "2px 8px", borderRadius: 2, whiteSpace: "nowrap"
    }}>
      {priorityShort(priority)}
    </span>
  );
}

function GroupBadge({ group, groupColor }) {
  if (!group || !groupColor) return null;
  const c = groupColor;
  return (
    <span style={{
      fontFamily: "'IBM Plex Mono', monospace", fontSize: 9, fontWeight: 600, letterSpacing: "0.1em", color: c,
      border: "1px solid color-mix(in oklch, " + c + " 40%, transparent)",
      background: "color-mix(in oklch, " + c + " 10%, transparent)",
      padding: "1px 5px", borderRadius: 2, whiteSpace: "nowrap"
    }}>
      {group}
    </span>
  );
}

function ScoreBar({ score }) {
  const color = score >= 90 ? "var(--must)" : score >= 75 ? "var(--should)" : "var(--text3)";
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 6, justifyContent: "flex-end" }}>
      <div style={{ width: 38, height: 3, background: "var(--border2)", borderRadius: 2, overflow: "hidden" }}>
        <div style={{ height: "100%", width: score + "%", background: color }} />
      </div>
      <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 10, color, minWidth: 22, textAlign: "right" }}>{score}</span>
    </div>
  );
}

function Nav({ index, total }) {
  const [time, setTime] = React.useState(() => new Date().toTimeString().slice(0, 8));
  React.useEffect(() => {
    const iv = setInterval(() => setTime(new Date().toTimeString().slice(0, 8)), 1000);
    return () => clearInterval(iv);
  }, []);
  return (
    <nav style={{
      position: "sticky", top: 0, zIndex: 100,
      background: "color-mix(in oklch, var(--bg) 90%, transparent)", backdropFilter: "blur(16px)",
      borderBottom: "1px solid var(--border)", padding: "0 24px", height: 56,
      display: "flex", alignItems: "center", justifyContent: "space-between"
    }}>
      <a href={DIGEST_HREF} style={{ display: "flex", alignItems: "center", gap: 0, textDecoration: "none" }}>
        <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontWeight: 600, fontSize: 16, letterSpacing: "0.14em", color: "var(--text)" }}>DOWN</span>
        <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontWeight: 600, fontSize: 16, letterSpacing: "0.14em", color: "var(--cyan)" }}>LINK</span>
        <span style={{
          marginLeft: 10, fontFamily: "'IBM Plex Mono', monospace", fontSize: 9, color: "var(--cyan)",
          letterSpacing: "0.1em", border: "1px solid color-mix(in oklch, var(--cyan) 30%, transparent)",
          background: "color-mix(in oklch, var(--cyan) 7%, transparent)", padding: "1px 6px", borderRadius: 2
        }}>TRIAGE</span>
      </a>
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <div className="live-dot" />
        <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 11, color: "var(--cyan)", letterSpacing: "0.08em" }}>LIVE</span>
        <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 11, color: "var(--text2)", letterSpacing: "0.06em" }}>{time}</span>
      </div>
      <div style={{
        fontFamily: "'IBM Plex Mono', monospace", fontSize: 10, color: "var(--text2)", letterSpacing: "0.08em",
        border: "1px solid var(--border2)", background: "var(--surface)", padding: "3px 10px", borderRadius: 2, whiteSpace: "nowrap"
      }}>
        {Math.min(index + 1, total)} / {total}
      </div>
    </nav>
  );
}

function CardFront({ article }) {
  const srcColor  = article.sourceColor;
  const priorityC = priorityColor(article.priority);
  return (
    <>
      <div style={{ height: 4, background: priorityC }} />
      <div style={{ padding: "20px 24px 16px", borderBottom: "1px solid var(--border)" }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 14 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <span style={{ width: 6, height: 6, borderRadius: "50%", background: srcColor, display: "inline-block" }} />
            <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 10, color: srcColor, letterSpacing: "0.04em" }}>
              {article.source}
            </span>
          </div>
          <div style={{ display: "flex", gap: 6, alignItems: "center" }}>
            {article.group && <GroupBadge group={article.group} groupColor={article.groupColor} />}
            <PriorityBadge priority={article.priority} />
          </div>
        </div>
        <h2 style={{ fontFamily: "'IBM Plex Sans', sans-serif", fontWeight: 500, fontSize: 22, lineHeight: 1.3, color: "var(--text)", letterSpacing: "-0.005em" }}>
          {article.title}
        </h2>
      </div>
      <div style={{ padding: "20px 24px", flex: 1, display: "flex", flexDirection: "column", overflow: "auto" }}>
        <span className="section-label" style={{ marginBottom: 12 }}>TL;DR</span>
        <p style={{ color: "var(--text2)", fontSize: 14, lineHeight: 1.65, fontWeight: 400 }}>{article.tldr}</p>
        {article.keyPoints && article.keyPoints.length > 0 && (
          <div style={{ marginTop: 18 }}>
            <span className="section-label" style={{ marginBottom: 10 }}>KEY POINTS</span>
            <ul style={{ listStyle: "none", marginTop: 10, display: "grid", gap: 8 }}>
              {article.keyPoints.map((kp, i) => (
                <li key={i} style={{ display: "grid", gridTemplateColumns: "14px 1fr", gap: 8, alignItems: "start" }}>
                  <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 10, color: priorityC, marginTop: 3, letterSpacing: "0.04em" }}>
                    {String(i + 1).padStart(2, "0")}
                  </span>
                  <span style={{ color: "var(--text)", fontSize: 13, lineHeight: 1.55 }}>{kp}</span>
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>
      <div style={{ padding: "14px 24px", borderTop: "1px solid var(--border)", display: "flex", justifyContent: "space-between", alignItems: "center", background: "var(--surface2)" }}>
        <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 10, color: "var(--text3)", letterSpacing: "0.06em" }}>{article.time}</span>
        <ScoreBar score={article.score} />
      </div>
    </>
  );
}

function CardBack({ article, onBack }) {
  const srcColor  = article.sourceColor;
  const priorityC = priorityColor(article.priority);
  const bodyHTML  = { __html: article.body };
  return (
    <>
      <div style={{ height: 4, background: priorityC }} />
      <div style={{ padding: "16px 24px", borderBottom: "1px solid var(--border)", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        <button onClick={onBack} style={{ all: "unset", cursor: "pointer", fontFamily: "'IBM Plex Mono', monospace", fontSize: 10, color: "var(--text2)", letterSpacing: "0.08em", display: "flex", alignItems: "center", gap: 6 }}>
          <span style={{ fontSize: 14 }}>&#8592;</span> BACK
        </button>
        <div style={{ display: "flex", gap: 6, alignItems: "center" }}>
          {article.group && <GroupBadge group={article.group} groupColor={article.groupColor} />}
          <PriorityBadge priority={article.priority} />
        </div>
      </div>
      <div className="detail-scroll" style={{ padding: "20px 24px", flex: 1 }}>
        <h2 style={{ fontFamily: "'IBM Plex Sans', sans-serif", fontWeight: 500, fontSize: 18, lineHeight: 1.35, color: "var(--text)", marginBottom: 12 }}>
          {article.title}
        </h2>
        <div style={{ display: "flex", gap: 8, alignItems: "center", marginBottom: 18, flexWrap: "wrap" }}>
          <span style={{ width: 5, height: 5, borderRadius: "50%", background: srcColor, display: "inline-block" }} />
          <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 10, color: srcColor }}>{article.source}</span>
          <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 10, color: "var(--text3)" }}>&#183;</span>
          <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 10, color: "var(--text3)" }}>{article.time}</span>
        </div>
        <div style={{ borderLeft: "2px solid color-mix(in oklch, " + priorityC + " 40%, transparent)", paddingLeft: 14, marginBottom: 18 }}>
          <span className="section-label" style={{ marginBottom: 8 }}>SUMMARY</span>
          <p style={{ color: "var(--text)", fontSize: 13, lineHeight: 1.75, marginTop: 8 }}>{article.tldr}</p>
        </div>
        <span className="section-label" style={{ marginBottom: 8 }}>FULL BRIEF</span>
        <div className="prose" style={{ marginTop: 10, marginBottom: 20 }} dangerouslySetInnerHTML={bodyHTML} />
        <a href={article.link} target="_blank" rel="noopener noreferrer"
          style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 10, color: "var(--cyan)", textDecoration: "none",
            letterSpacing: "0.08em", padding: "6px 12px",
            border: "1px solid color-mix(in oklch, var(--cyan) 30%, transparent)", borderRadius: 2, display: "inline-block" }}>
          OPEN SOURCE &#8599;
        </a>
      </div>
      <div style={{ padding: "12px 24px", borderTop: "1px solid var(--border)", background: "var(--surface2)", display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 10, color: "var(--text3)", letterSpacing: "0.06em" }}>
          ART · {String(article.n).padStart(2, "0")}
        </span>
        <ScoreBar score={article.score} />
      </div>
    </>
  );
}

function SwipeCard({ article, isTop, depth, onSwipe }) {
  const [dx, setDx]             = React.useState(0);
  const [dy, setDy]             = React.useState(0);
  const [dragging, setDragging] = React.useState(false);
  const [flying, setFlying]     = React.useState(null);
  const [flipped, setFlipped]   = React.useState(false);
  const startRef = React.useRef({ x: 0, y: 0 });

  React.useEffect(() => {
    if (isTop && !flipped) {
      window.__downlinkFlipTop = () => setFlipped(true);
    }
    return () => { if (isTop) window.__downlinkFlipTop = null; };
  }, [isTop, flipped]);

  const onPointerDown = (e) => {
    if (!isTop || flipped || flying) return;
    setDragging(true);
    startRef.current = { x: e.clientX, y: e.clientY };
    e.target.setPointerCapture && e.target.setPointerCapture(e.pointerId);
  };
  const onPointerMove = (e) => {
    if (!dragging) return;
    setDx(e.clientX - startRef.current.x);
    setDy(e.clientY - startRef.current.y);
  };
  const onPointerUp = () => {
    if (!dragging) return;
    setDragging(false);
    const threshold = 110;
    if (dx > threshold) {
      setFlying("right");
      setTimeout(() => { setFlipped(true); setDx(0); setDy(0); setFlying(null); }, 280);
    } else if (dx < -threshold) {
      setFlying("left");
      setTimeout(() => onSwipe("left"), 380);
    } else {
      setDx(0); setDy(0);
    }
  };

  let transform; let opacity = 1;
  if (flying === "left")       { transform = "translate(-140%, " + (dy * 1.5) + "px) rotate(-22deg)"; opacity = 0; }
  else if (flying === "right") { transform = "translate(60%, " + dy + "px) rotate(12deg) scale(0.95)"; }
  else if (dragging)           { transform = "translate(" + dx + "px, " + (dy * 0.4) + "px) rotate(" + (dx * 0.05) + "deg)"; }
  else if (depth > 0)          { transform = "translateY(" + (depth * 8) + "px) scale(" + (1 - depth * 0.04) + ")"; }
  else                         { transform = "translate(0, 0) rotate(0)"; }

  const skipOpacity = !flipped && dx < 0 ? Math.min(1, -dx / 100) : 0;
  const readOpacity = !flipped && dx > 0 ? Math.min(1, dx / 100) : 0;
  const cardClass   = "swipe-card " + (dragging ? "dragging" : flying ? "fly-out" : "snap-back");

  return (
    <div className={cardClass}
      style={{ transform, opacity, zIndex: 100 - depth, cursor: isTop && !flipped ? "grab" : "default", pointerEvents: isTop ? "auto" : "none" }}
      onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={onPointerUp} onPointerCancel={onPointerUp}>
      {!flipped ? <CardFront article={article} /> : <CardBack article={article} onBack={() => setFlipped(false)} />}
      {!flipped && (
        <>
          <div className="stamp skip" style={{ opacity: skipOpacity }}>SKIP</div>
          <div className="stamp read" style={{ opacity: readOpacity }}>READ</div>
        </>
      )}
    </div>
  );
}

function App() {
  const [index, setIndex] = React.useState(0);
  const [tick, setTick]   = React.useState(0);
  const total     = ARTICLES.length;
  const remaining = total - index;
  const finished  = index >= total;

  React.useEffect(() => {
    const onKey = (e) => {
      if (finished) return;
      if (e.key === "ArrowLeft")  { setIndex(i => i + 1); setTick(t => t + 1); }
      if (e.key === "ArrowRight") { if (window.__downlinkFlipTop) window.__downlinkFlipTop(); }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [finished]);

  const advance = () => { setIndex(i => i + 1); setTick(t => t + 1); };

  return (
    <div style={{ minHeight: "100vh" }}>
      <Nav index={index} total={total} />
      {finished ? (
        <div style={{ display: "flex", justifyContent: "center", padding: "80px 24px 0" }}>
          <div style={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: 8, padding: "48px 40px", textAlign: "center", maxWidth: 460, width: "100%" }}>
            <div style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 11, color: "var(--cyan)", letterSpacing: "0.18em", marginBottom: 16 }}>// QUEUE EMPTY</div>
            <h2 style={{ fontFamily: "'IBM Plex Sans', sans-serif", fontWeight: 500, fontSize: 24, marginBottom: 10 }}>You're caught up.</h2>
            <p style={{ color: "var(--text2)", fontSize: 13, lineHeight: 1.7, marginBottom: 24 }}>
              All {total} articles in the {TIME_WINDOW} window have been triaged.
            </p>
            <button onClick={() => { setIndex(0); setTick(0); }}
              style={{ all: "unset", cursor: "pointer", fontFamily: "'IBM Plex Mono', monospace", fontSize: 11, color: "var(--cyan)", letterSpacing: "0.08em", padding: "10px 20px", border: "1px solid color-mix(in oklch, var(--cyan) 35%, transparent)", borderRadius: 2, background: "color-mix(in oklch, var(--cyan) 8%, transparent)" }}>
              &#8635; REVIEW AGAIN
            </button>
            <div style={{ marginTop: 20 }}>
              <a href={DIGEST_HREF} style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 10, color: "var(--text3)", letterSpacing: "0.08em", textDecoration: "none", borderBottom: "1px dotted var(--text3)" }}>
                &#8592; BACK TO LIST
              </a>
            </div>
          </div>
        </div>
      ) : (
        <div style={{ padding: "32px 24px 48px" }}>
          <div style={{ maxWidth: 520, margin: "0 auto 20px" }}>
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 8 }}>
              <span className="section-label">TRIAGE QUEUE</span>
              <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 10, color: "var(--text3)", letterSpacing: "0.06em" }}>
                {remaining} REMAINING · {index} CLEARED
              </span>
            </div>
            <div className="progress-bar"><div style={{ width: ((index / total) * 100) + "%" }} /></div>
          </div>

          <div className="card-stack">
            {ARTICLES.slice(index, index + 3).slice().reverse().map((article, revI, arr) => {
              const realDepth = arr.length - 1 - revI;
              return (
                <SwipeCard
                  key={article.n + "-" + tick + "-" + realDepth}
                  article={article}
                  isTop={realDepth === 0}
                  depth={realDepth}
                  onSwipe={(dir) => { if (dir === "left") advance(); }}
                />
              );
            })}
          </div>

          <div style={{ display: "flex", gap: 28, justifyContent: "center", alignItems: "center", marginTop: 32 }}>
            <button className="pill-btn skip" onClick={advance} title="Skip (ArrowLeft)">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
              </svg>
            </button>
            <div style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 9, color: "var(--text3)", letterSpacing: "0.12em", textAlign: "center", lineHeight: 1.7 }}>
              <div>&#8592; SKIP · READ &#8594;</div>
              <div style={{ opacity: 0.6, marginTop: 2 }}>SWIPE OR CLICK</div>
            </div>
            <button className="pill-btn read" onClick={() => { if (window.__downlinkFlipTop) window.__downlinkFlipTop(); }} title="Read details (ArrowRight)">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                <polyline points="20 6 9 17 4 12" />
              </svg>
            </button>
          </div>

          <div style={{ marginTop: 32, textAlign: "center" }}>
            <a href={DIGEST_HREF} style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 10, color: "var(--text3)", letterSpacing: "0.08em", textDecoration: "none", borderBottom: "1px dotted var(--text3)" }}>
              &#8592; BACK TO LIST VIEW
            </a>
          </div>
        </div>
      )}
    </div>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<App />);
</script>
<div id="root"></div>
</body>
</html>
`
