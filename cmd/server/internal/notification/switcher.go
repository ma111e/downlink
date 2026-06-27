package notification

import (
	"bytes"
	"fmt"

	"github.com/ma111e/downlink/pkg/utils"
)

// profileSwitcherSnippet returns a self-contained floating profile switcher: a
// fixed-position control that fetches profiles.json at the repo root and links
// to each profile's section. It needs no layout markup, so it works on every
// layout (including custom on-disk packs).
//
// rootPrefix is the relative path from the current page to the repo root (e.g.
// "../"); currentSlug marks the active profile. Both are baked in as JSON-safe
// string literals.
func profileSwitcherSnippet(currentSlug, rootPrefix string) string {
	// %q produces safe double-quoted Go/JS string literals for the two values.
	// switcherCSS is injected as a plain argument, so its literal % characters
	// need no escaping (unlike when the CSS lived in the format string).
	return fmt.Sprintf(`<div id="dl-profile-switcher" hidden></div>
<style>%s</style>
<script>
(function(){
  var ROOT=%q, CUR=%q;
  fetch(ROOT+"profiles.json").then(function(r){return r.ok?r.json():null;}).then(function(d){
    if(!d||!d.profiles||d.profiles.length<2)return;
    var box=document.getElementById("dl-profile-switcher");
    var sel=document.createElement("select");
    sel.setAttribute("aria-label","Switch profile");
    d.profiles.forEach(function(p){
      var o=document.createElement("option");
      o.value=ROOT+p.subdir+"/";
      o.textContent=(p.icon?p.icon+" ":"")+(p.name||p.slug);
      if(p.slug===CUR)o.selected=true;
      sel.appendChild(o);
    });
    sel.addEventListener("change",function(){if(sel.value)location.href=sel.value;});
    box.appendChild(sel);
    box.hidden=false;
  }).catch(function(){});
})();
</script>`, switcherCSS, rootPrefix, currentSlug)
}

// switcherCSS is the floating profile-switcher stylesheet, split out into
// templates/switcher.css and inlined into the snippet at injection time.
var switcherCSS = func() string {
	b, err := notificationTemplateFS.ReadFile("templates/switcher.css")
	if err != nil {
		panic(fmt.Sprintf("read embedded switcher.css: %v", err))
	}
	return utils.StripCSSComments(string(b))
}()

// injectProfileSwitcher inserts the switcher snippet just before the last
// </body> tag, falling back to appending it when no such tag is present.
func injectProfileSwitcher(html []byte, slug, rootPrefix string) []byte {
	snippet := []byte(profileSwitcherSnippet(slug, rootPrefix))
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
