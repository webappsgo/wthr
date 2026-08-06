package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// newNotificationTemplateTestHandler wires a NotificationTemplateHandler
// against a fresh legacy-schema in-memory DB. notification_templates only
// exists in database.Schema (legacy), the same schema-mismatch pattern
// documented for notification_channels.go / notification_preferences.go.
func newNotificationTemplateTestHandler(t *testing.T) *NotificationTemplateHandler {
	t.Helper()
	db := newNotificationChannelsTestDB(t)
	return NewNotificationTemplateHandler(db)
}

func insertTestTemplate(t *testing.T, h *NotificationTemplateHandler, channelType, name, tmplType, subject, body string, isDefault bool) int64 {
	t.Helper()
	res, err := h.DB.Exec(`
		INSERT INTO notification_templates
		(channel_type, template_name, template_type, subject_template, body_template, is_default, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`, channelType, name, tmplType, subject, body, isDefault)
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
		insertTestTemplate(t, h, "email", "welcome", "general", "Hi {{.Name}}", "Body", true)
		insertTestTemplate(t, h, "sms", "alert", "alert", "", "Alert!", false)

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
		insertTestTemplate(t, h, "email", "welcome", "general", "Hi", "Body", true)
		insertTestTemplate(t, h, "sms", "alert", "alert", "", "Alert!", false)

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
		id := insertTestTemplate(t, h, "email", "welcome", "general", "Hi", "Body", true)

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
			"is_default":       true,
		})

		h.CreateTemplate(c)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
		}

		var count int
		if err := h.DB.QueryRow("SELECT COUNT(*) FROM notification_templates WHERE template_name = 'welcome'").Scan(&count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 row, got %d", count)
		}
	})

	t.Run("setting is_default clears other defaults for the channel", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		insertTestTemplate(t, h, "email", "old-default", "general", "", "Body", true)

		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/templates", map[string]interface{}{
			"channel_type":  "email",
			"template_name": "new-default",
			"template_type": "general",
			"body_template": "Body",
			"is_default":    true,
		})

		h.CreateTemplate(c)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
		}

		var oldIsDefault bool
		if err := h.DB.QueryRow("SELECT is_default FROM notification_templates WHERE template_name = 'old-default'").Scan(&oldIsDefault); err != nil {
			t.Fatalf("query old default: %v", err)
		}
		if oldIsDefault {
			t.Errorf("expected old default to be cleared")
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
		insertTestTemplate(t, h, "email", "welcome", "general", "", "Body", false)

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
		insertTestTemplate(t, h, "email", "welcome", "general", "", "Old body", false)

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
		if err := h.DB.QueryRow("SELECT template_name, body_template FROM notification_templates WHERE id = 1").Scan(&name, &body); err != nil {
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
		insertTestTemplate(t, h, "email", "welcome", "general", "", "Body", true)

		c, w := newAPITestContext("/notifications/templates/1")
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.DeleteTemplate(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("non-default template is deleted", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		insertTestTemplate(t, h, "email", "welcome", "general", "", "Body", false)

		c, w := newAPITestContext("/notifications/templates/1")
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.DeleteTemplate(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}

		var count int
		if err := h.DB.QueryRow("SELECT COUNT(*) FROM notification_templates WHERE id = 1").Scan(&count); err != nil {
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

	t.Run("valid clone creates a non-default copy", func(t *testing.T) {
		h := newNotificationTemplateTestHandler(t)
		insertTestTemplate(t, h, "email", "welcome", "general", "Hi", "Body", true)

		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/templates/1/clone", map[string]interface{}{
			"new_name": "welcome-copy",
		})
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.CloneTemplate(c)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
		}

		var isDefault bool
		if err := h.DB.QueryRow("SELECT is_default FROM notification_templates WHERE template_name = 'welcome-copy'").Scan(&isDefault); err != nil {
			t.Fatalf("query clone: %v", err)
		}
		if isDefault {
			t.Errorf("expected cloned template to not be default")
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
	if err := h.DB.QueryRow("SELECT COUNT(*) FROM notification_templates").Scan(&count); err != nil {
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
