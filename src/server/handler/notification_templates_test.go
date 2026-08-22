package handler

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/server/service"
)

// newNotificationTemplateTestHandler wires a NotificationTemplateHandler
// against a fresh in-memory DB built from the real production
// database.ServerSchema, which is the only definition of
// server_notification_templates the handler uses.
func newNotificationTemplateTestHandler(t *testing.T) *NotificationTemplateHandler {
	t.Helper()
	db := newNotificationChannelsTestDB(t)
	return NewNotificationTemplateHandler(db)
}

// insertTestTemplate seeds one row of server_notification_templates. The table
// carries no is_default column: a channel's default template is the one named
// service.DefaultTemplateName, so callers make a seed the default by passing
// that name.
func insertTestTemplate(t *testing.T, h *NotificationTemplateHandler, channelType, name, tmplType, subject, body string) int64 {
	t.Helper()
	now := dbtime.FormatSQLTimestamp(time.Now())
	res, err := h.DB.Exec(`
		INSERT INTO server_notification_templates
		(channel_type, template_name, template_type, subject, body, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, channelType, name, tmplType, subject, body, now, now)
	if err != nil {
		t.Fatalf("insert template: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestNotificationTemplateHandlerListTemplates(t *testing.T) {
	t.Run("empty table returns empty list", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		c, w := newAPITestContext("/notifications/templates")

		h.ListTemplates(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"total":0`) {
			t.Errorf("expected total:0, got: %s", w.Body.String())
		}
	})

	t.Run("returns grouped templates", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		insertTestTemplate(t, h, "email", "welcome", "general", "Hi {{.Name}}", "Body")
		insertTestTemplate(t, h, "sms", "alert", "alert", "", "Alert!")

		c, w := newAPITestContext("/notifications/templates")

		h.ListTemplates(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, `"total":2`) {
			t.Errorf("expected total:2, got: %s", body)
		}
		if !strings.Contains(body, `"grouped"`) {
			t.Errorf("expected grouped key, got: %s", body)
		}
	})

	t.Run("filters by channel_type", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		insertTestTemplate(t, h, "email", "welcome", "general", "Hi", "Body")
		insertTestTemplate(t, h, "sms", "alert", "alert", "", "Alert!")

		c, w := newAPITestContext("/notifications/templates?channel_type=sms")

		h.ListTemplates(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, `"total":1`) {
			t.Errorf("expected total:1, got: %s", body)
		}
		if !strings.Contains(body, `"channel_type":"sms"`) {
			t.Errorf("expected sms template, got: %s", body)
		}
	})
}

func TestNotificationTemplateHandlerGetTemplate(t *testing.T) {
	t.Run("unknown id returns 404", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		c, w := newAPITestContext("/notifications/templates/999")
		c.Params = gin.Params{{Key: "id", Value: "999"}}

		h.GetTemplate(c)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404: %s", w.Code, w.Body.String())
		}
	})

	t.Run("existing template returns its fields", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		id := insertTestTemplate(t, h, "email", "welcome", "general", "Hi", "Body")

		c, w := newAPITestContext("/notifications/templates/1")
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.GetTemplate(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, `"template_name":"welcome"`) {
			t.Errorf("expected template_name welcome, got: %s", body)
		}
		_ = id
	})
}

func TestNotificationTemplateHandlerCreateTemplate(t *testing.T) {
	t.Run("malformed body returns 400", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/templates", "not json")

		h.CreateTemplate(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing required fields returns 400", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/templates", map[string]interface{}{
			"channel_type": "email",
		})

		h.CreateTemplate(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid body template syntax returns 400", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/templates", map[string]interface{}{
			"channel_type":  "email",
			"template_name": "broken",
			"template_type": "general",
			"body_template": "{{.Unclosed",
		})

		h.CreateTemplate(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("valid template is created", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/templates", map[string]interface{}{
			"channel_type":     "email",
			"template_name":    "welcome",
			"template_type":    "general",
			"subject_template": "Hi {{.Name}}",
			"body_template":    "Welcome, {{.Name}}!",
		})

		h.CreateTemplate(c)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
		}

		var count int
		if err := h.DB.QueryRow("SELECT COUNT(*) FROM server_notification_templates WHERE template_name = 'welcome'").Scan(&count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 row, got %d", count)
		}
	})

	// Default-ness is derived from the reserved template name rather than
	// stored, so a template created under that name reads back as the default.
	t.Run("template created under the reserved name reads back as default", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/templates", map[string]interface{}{
			"channel_type":  "email",
			"template_name": service.DefaultTemplateName,
			"template_type": "general",
			"body_template": "Body",
		})

		h.CreateTemplate(c)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
		}

		c, w = newAPITestContext("/notifications/templates/1")
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.GetTemplate(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"is_default":true`) {
			t.Errorf("expected is_default:true, got: %s", w.Body.String())
		}
	})
}

func TestNotificationTemplateHandlerUpdateTemplate(t *testing.T) {
	t.Run("malformed body returns 400", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPut, "/notifications/templates/1", "not json")
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.UpdateTemplate(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unknown id returns 404", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPut, "/notifications/templates/999", map[string]interface{}{
			"template_name": "x",
		})
		c.Params = gin.Params{{Key: "id", Value: "999"}}

		h.UpdateTemplate(c)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid body template syntax returns 400", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		insertTestTemplate(t, h, "email", "welcome", "general", "", "Body")

		c, w := newTestContextJSON(t, http.MethodPut, "/notifications/templates/1", map[string]interface{}{
			"body_template": "{{.Unclosed",
		})
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.UpdateTemplate(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("valid update persists fields", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		insertTestTemplate(t, h, "email", "welcome", "general", "", "Old body")

		c, w := newTestContextJSON(t, http.MethodPut, "/notifications/templates/1", map[string]interface{}{
			"template_name": "welcome-v2",
			"template_type": "general",
			"body_template": "New body",
		})
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.UpdateTemplate(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}

		var name, body string
		if err := h.DB.QueryRow("SELECT template_name, body FROM server_notification_templates WHERE id = 1").Scan(&name, &body); err != nil {
			t.Fatalf("query updated row: %v", err)
		}
		if name != "welcome-v2" || body != "New body" {
			t.Errorf("name=%q body=%q, want welcome-v2/New body", name, body)
		}
	})
}

func TestNotificationTemplateHandlerDeleteTemplate(t *testing.T) {
	t.Run("unknown id returns 404", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		c, w := newAPITestContext("/notifications/templates/999")
		c.Params = gin.Params{{Key: "id", Value: "999"}}

		h.DeleteTemplate(c)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404: %s", w.Code, w.Body.String())
		}
	})

	t.Run("default template cannot be deleted", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		insertTestTemplate(t, h, "email", service.DefaultTemplateName, "general", "", "Body")

		c, w := newAPITestContext("/notifications/templates/1")
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.DeleteTemplate(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("non-default template is deleted", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		insertTestTemplate(t, h, "email", "welcome", "general", "", "Body")

		c, w := newAPITestContext("/notifications/templates/1")
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.DeleteTemplate(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}

		var count int
		if err := h.DB.QueryRow("SELECT COUNT(*) FROM server_notification_templates WHERE id = 1").Scan(&count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("expected row deleted, still present")
		}
	})
}

func TestNotificationTemplateHandlerPreviewTemplate(t *testing.T) {
	t.Run("malformed body returns 400", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/templates/preview", "not json")

		h.PreviewTemplate(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing body_template returns 400", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/templates/preview", map[string]interface{}{})

		h.PreviewTemplate(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unresolvable subject template returns 400", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/templates/preview", map[string]interface{}{
			"subject_template": "{{.Unclosed",
			"body_template":    "Hello",
		})

		h.PreviewTemplate(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("renders subject and body with variables", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/templates/preview", map[string]interface{}{
			"subject_template": "Hi {{.Name}}",
			"body_template":    "Welcome, {{.Name}}!",
			"variables":        map[string]interface{}{"Name": "Ada"},
		})

		h.PreviewTemplate(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, "Hi Ada") || !strings.Contains(body, "Welcome, Ada!") {
			t.Errorf("expected rendered variables, got: %s", body)
		}
	})
}

func TestNotificationTemplateHandlerCloneTemplate(t *testing.T) {
	t.Run("malformed body returns 400", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/templates/1/clone", "not json")
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.CloneTemplate(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing new_name returns 400", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/templates/1/clone", map[string]interface{}{})
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.CloneTemplate(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unknown source id returns 404", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/templates/999/clone", map[string]interface{}{
			"new_name": "copy",
		})
		c.Params = gin.Params{{Key: "id", Value: "999"}}

		h.CloneTemplate(c)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404: %s", w.Code, w.Body.String())
		}
	})

	t.Run("valid clone copies the source under the new name", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		insertTestTemplate(t, h, "email", "welcome", "general", "Hi", "Body")

		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/templates/1/clone", map[string]interface{}{
			"new_name": "welcome-copy",
		})
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.CloneTemplate(c)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
		}

		var clonedBody string
		if err := h.DB.QueryRow("SELECT body FROM server_notification_templates WHERE template_name = 'welcome-copy'").Scan(&clonedBody); err != nil {
			t.Fatalf("query clone: %v", err)
		}
		if clonedBody != "Body" {
			t.Errorf("cloned body = %q, want %q", clonedBody, "Body")
		}
	})

	t.Run("cloning onto the reserved default name returns 400", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		insertTestTemplate(t, h, "email", "welcome", "general", "Hi", "Body")

		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/templates/1/clone", map[string]interface{}{
			"new_name": service.DefaultTemplateName,
		})
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.CloneTemplate(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})
}

func TestNotificationTemplateHandlerInitializeDefaults(t *testing.T) {
	h := newNotificationTemplateTestHandler(t)
	c, w := newAPITestContext("/notifications/templates/init")

	h.InitializeDefaults(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}

	var count int
	if err := h.DB.QueryRow("SELECT COUNT(*) FROM server_notification_templates").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count == 0 {
		t.Errorf("expected default templates to be inserted")
	}
}

func TestNotificationTemplateHandlerGetTemplateVariables(t *testing.T) {
	h := newNotificationTemplateTestHandler(t)
	c, w := newAPITestContext("/notifications/templates/variables")

	h.GetTemplateVariables(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, key := range []string{"common", "weather", "system", "functions"} {
		if !strings.Contains(body, key) {
			t.Errorf("expected %q key in response, got: %s", key, body)
		}
	}
}
