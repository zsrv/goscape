package ondemand

import (
	"embed"
	"fmt"
	"html/template"
)

// templatesFS embeds the rs2.cgi HTML bootstrap templates. They are ports of
// Engine-TS's view/client.ejs and view/java.ejs to Go html/template syntax
// (EJS <%= var %> -> {{.Var}}); see rs2cgi.go for the request dispatch.
//
//go:embed templates/client.html templates/java.html
var templatesFS embed.FS

// rs2cgiTemplates is the parsed template set, indexed by base name
// ("client.html", "java.html"). Parsed once at package init so a malformed
// embedded template surfaces as a fatal startup error rather than a runtime
// 500 on the first /rs2.cgi request.
var rs2cgiTemplates = func() *template.Template {
	t, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		panic(fmt.Errorf("ondemand: parse rs2.cgi templates: %w", err))
	}
	return t
}()
