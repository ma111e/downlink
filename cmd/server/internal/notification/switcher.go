package notification

import (
	"bytes"
	"fmt"
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
	return fmt.Sprintf(`<div id="dl-profile-switcher" hidden></div>
<style>
#dl-profile-switcher{position:fixed;top:12px;right:12px;z-index:2147483000;font:13px/1.4 system-ui,sans-serif}
#dl-profile-switcher select{appearance:none;cursor:pointer;padding:.4em 1.8em .4em .7em;border-radius:8px;
  border:1px solid rgba(127,127,127,.4);background:rgba(127,127,127,.12);color:inherit;
  background-image:url("data:image/svg+xml,%%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6'%%3E%%3Cpath d='M0 0l5 6 5-6z' fill='%%23999'/%%3E%%3C/svg%%3E");
  background-repeat:no-repeat;background-position:right .6em center;backdrop-filter:blur(6px)}
@media print{#dl-profile-switcher{display:none}}
</style>
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
</script>`, rootPrefix, currentSlug)
}

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
