package notification

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/ma111e/downlink/pkg/utils"
)

// defaultLayoutName is the classic layout: the one that carries the "try the
// new layout" call-to-action and is the target of the v2 settings toggle.
const defaultLayoutName = "default"

// layoutSwitchStorageKey persists the reader's chosen layout name; a matching
// value redirects them to that layout on every page. dismissedKey suppresses the
// call-to-action once dismissed. Both follow the page's downlink.* convention.
const (
	layoutSwitchStorageKey = "downlink.layout"
	layoutSwitchDismissKey = "downlink.layout.dismissed"
)

// pickLatestLayout returns the layout to advertise on the classic page: the
// redesigned layout a reader is invited to try. It prefers "v2", else the first
// non-default peer. Returns "" when only the default layout is present.
func pickLatestLayout(current string, peers []LayoutPeer) string {
	latest := ""
	for _, pe := range peers {
		if pe.Layout == defaultLayoutName || pe.Layout == current {
			continue
		}
		if pe.Layout == "v2" {
			return "v2"
		}
		if latest == "" {
			latest = pe.Layout
		}
	}
	return latest
}

// layoutPeerMap maps each peer layout name to its output subdir.
func layoutPeerMap(peers []LayoutPeer) map[string]string {
	m := make(map[string]string, len(peers))
	for _, pe := range peers {
		m[pe.Layout] = pe.Subdir
	}
	return m
}

// layoutHeadSnippet returns the pre-paint script injected into <head> on every
// layout. It exposes window.__dlLayout (used by the CTA banner and the v2
// settings toggle) and redirects the reader to their remembered layout before
// first paint by swapping the current layout's subdir segment in the URL.
func layoutHeadSnippet(current, currentSubdir string, peersJSON []byte) string {
	return fmt.Sprintf(`<script>
(function(){
  var CUR=%q, CUR_SUB=%q, PEERS=%s;
  function swap(tgtSub){
    var p=location.pathname, n="/"+CUR_SUB+"/", i=p.indexOf(n);
    if(i<0) return null;
    return p.slice(0,i)+"/"+tgtSub+"/"+p.slice(i+n.length)+location.search+location.hash;
  }
  window.__dlLayout={
    current:CUR, peers:PEERS,
    has:function(l){return !!(PEERS[l]&&l!==CUR);},
    go:function(l){ if(!PEERS[l]||l===CUR)return;
      try{localStorage.setItem(%q,l);}catch(e){}
      var t=swap(PEERS[l]); if(t)location.href=t; }
  };
  try{
    var pref=localStorage.getItem(%q);
    if(pref&&pref!==CUR&&PEERS[pref]){ var t=swap(PEERS[pref]); if(t){location.replace(t);} }
  }catch(e){}
})();
</script>`, current, currentSubdir, peersJSON, layoutSwitchStorageKey, layoutSwitchStorageKey)
}

// layoutCTASnippet returns the floating "try the new layout" banner injected
// before </body> on the classic (default) layout. It self-hides once the reader
// has picked a layout or dismissed it, and drives navigation through
// window.__dlLayout defined by the head snippet.
func layoutCTASnippet(latest string) string {
	return fmt.Sprintf(`<div id="dl-layout-cta" hidden>
<span style="display:none !important">A redesigned layout is available.</span>
<button type="button" data-dl-try>Try it</button>
<button type="button" data-dl-dismiss>Not now</button>
</div>
<style>%s</style>
<script>
(function(){
  try{
    if(localStorage.getItem(%q)) return;
    if(localStorage.getItem(%q)) return;
  }catch(e){}
  var box=document.getElementById("dl-layout-cta"); if(!box)return;
  box.hidden=false;
  box.querySelector("[data-dl-try]").addEventListener("click",function(){
    if(window.__dlLayout) window.__dlLayout.go(%q);
  });
  box.querySelector("[data-dl-dismiss]").addEventListener("click",function(){
    try{localStorage.setItem(%q,"1");}catch(e){} box.hidden=true;
  });
})();
</script>`, layoutSwitchCSS, layoutSwitchStorageKey, layoutSwitchDismissKey, latest, layoutSwitchDismissKey)
}

// layoutSwitchCSS is the CTA banner stylesheet, split into
// templates/layoutswitch.css and inlined into the snippet at injection time.
var layoutSwitchCSS = func() string {
	b, err := notificationTemplateFS.ReadFile("templates/layoutswitch.css")
	if err != nil {
		panic(fmt.Sprintf("read embedded layoutswitch.css: %v", err))
	}
	return utils.StripCSSComments(string(b))
}()

// injectLayoutSwitch adds the layout-switch UI to a rendered page: the pre-paint
// redirect/API script into <head> (all layouts), plus the "try the new layout"
// call-to-action before </body> on the classic (default) layout. The caller
// guarantees len(peers) > 1.
func injectLayoutSwitch(html []byte, currentLayout string, peers []LayoutPeer) []byte {
	peerMap := layoutPeerMap(peers)
	currentSubdir, ok := peerMap[currentLayout]
	if !ok {
		return html
	}
	peersJSON, err := json.Marshal(peerMap)
	if err != nil {
		return html
	}

	out := insertIntoHead(html, []byte(layoutHeadSnippet(currentLayout, currentSubdir, peersJSON)))

	if currentLayout == defaultLayoutName {
		if latest := pickLatestLayout(currentLayout, peers); latest != "" {
			out = insertBeforeBodyClose(out, []byte(layoutCTASnippet(latest)))
		}
	}
	return out
}

// insertIntoHead places snippet immediately after the opening <head> tag so it
// executes before the body renders. Falls back to prepending when no <head> is
// present.
func insertIntoHead(html, snippet []byte) []byte {
	marker := []byte("<head>")
	idx := bytes.Index(html, marker)
	if idx == -1 {
		return append(append([]byte{}, snippet...), html...)
	}
	at := idx + len(marker)
	out := make([]byte, 0, len(html)+len(snippet))
	out = append(out, html[:at]...)
	out = append(out, snippet...)
	out = append(out, html[at:]...)
	return out
}

// insertBeforeBodyClose places snippet just before the last </body>, falling
// back to appending when no such tag is present. Mirrors injectProfileSwitcher.
func insertBeforeBodyClose(html, snippet []byte) []byte {
	marker := []byte("</body>")
	if idx := bytes.LastIndex(html, marker); idx != -1 {
		out := make([]byte, 0, len(html)+len(snippet))
		out = append(out, html[:idx]...)
		out = append(out, snippet...)
		out = append(out, html[idx:]...)
		return out
	}
	return append(html, snippet...)
}
