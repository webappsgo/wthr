package handler

import (
	"html"
	"net/http"
	"strings"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/util"
)

// MCPToolInfo is one public MCP tool as shown on the MCP help page.
type MCPToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// MCPHelpSource is what the MCP help page needs from the MCP server.
type MCPHelpSource interface {
	Enabled() bool
	PublicToolList() []MCPToolInfo
}

// dpoData builds the data shared by the /server/dpo page and its JSON mirror (AI.md PART 16
// "Compliance Routes"). The email falls back to the general address and never to the admin
// address; privacy_url is absolute and built from the request, never cached.
func dpoData(r *http.Request, cfg *config.AppConfig) map[string]interface{} {
	name, email := "", ""
	if cfg != nil {
		name = strings.TrimSpace(cfg.Server.Contact.DPO.Name)
		email = cfg.Server.Contact.ResolveDPOEmail()
	}
	return map[string]interface{}{
		"name":        name,
		"email":       email,
		"privacy_url": util.BuildURL(r, "/server/privacy"),
	}
}

// ShowDPOPage renders the Data Protection Officer contact page.
// GET /server/dpo
func ShowDPOPage(cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dpo := dpoData(r, cfg)
		data := map[string]interface{}{
			"page":     "dpo",
			"server":   pageServerData(r, map[string]interface{}{"Title": cfg.Server.Branding.Title}),
			"HostInfo": util.GetHostInfo(r),
			"DPO":      dpo,
			"name":     dpo["name"],
			"email":    dpo["email"],
			// Root-relative link for the HTML page; the JSON mirror carries the absolute one
			"privacy_url": "/server/privacy",
		}
		NegotiateResponse(w, r, "page/dpo.tmpl", data)
	}
}

// GetDPOAPI is the JSON mirror of the DPO page. It always answers 200: when no address
// resolves, email is "" (never 404).
// GET /api/{api_version}/server/dpo
func GetDPOAPI(cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		RespondNegotiatedData(w, r, http.StatusOK, dpoData(r, cfg))
	}
}

// mcpHelpData builds the MCP help content: base sections are fixed, the project section comes
// from pages.help.mcp. Every URL is resolved per request (AI.md PART 8 "Resolution Order").
func mcpHelpData(r *http.Request, cfg *config.AppConfig, src MCPHelpSource) map[string]interface{} {
	apiPath := cfg.GetAPIPath()
	tools := src.PublicToolList()
	if tools == nil {
		tools = []MCPToolInfo{}
	}
	return map[string]interface{}{
		"enabled": true,
		"url":     util.BuildURL(r, apiPath+"/server/mcp"),
		"aliases": []string{util.BuildURL(r, "/api/mcp"), util.BuildURL(r, "/mcp")},
		"auth":    map[string]string{"scheme": "Bearer"},
		"tools":   tools,
		"project": cfg.Server.Pages.Help.MCP,
	}
}

// ShowMCPHelpPage renders how to connect an AI assistant to this application. It answers 404
// when MCP is disabled.
// GET /server/help/mcp
func ShowMCPHelpPage(cfg *config.AppConfig, src MCPHelpSource) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if src == nil || !src.Enabled() {
			http.NotFound(w, r)
			return
		}
		content := mcpHelpData(r, cfg, src)
		data := map[string]interface{}{
			"page":     "help_mcp",
			"server":   pageServerData(r, map[string]interface{}{"Title": cfg.Server.Branding.Title}),
			"HostInfo": util.GetHostInfo(r),
			"MCP":      content,
			"Project":  html.EscapeString(cfg.Server.Pages.Help.MCP),
			"MCP_name": util.ProjectName,
		}
		NegotiateResponse(w, r, "page/help_mcp.tmpl", data)
	}
}

// GetMCPHelpAPI is the JSON mirror of the MCP help page.
// GET /api/{api_version}/server/help/mcp
func GetMCPHelpAPI(cfg *config.AppConfig, src MCPHelpSource) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if src == nil || !src.Enabled() {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", Translate(r, "error.not_found"))
			return
		}
		RespondNegotiatedData(w, r, http.StatusOK, map[string]interface{}{"ok": true, "data": mcpHelpData(r, cfg, src)})
	}
}
