package scheduler

import (
	"context"
	"errors"

	"github.com/webappsgo/wthr/src/server/service"
)

// RegisterContactWebhookRetryTask installs the global retry worker. The task
// runs on one cluster node and uses the durable queue in server.db.
func RegisterContactWebhookRetryTask(s *Scheduler) error {
	if s == nil {
		return errors.New("scheduler is required")
	}
	return s.AddTask("contact_webhook_retry", "@every 1m", func() error {
		return service.ProcessContactWebhookRetries(context.Background(), s.db)
	})
}
