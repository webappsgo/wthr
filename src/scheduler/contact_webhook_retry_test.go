package scheduler

import "testing"

func TestRegisterContactWebhookRetryTask(t *testing.T) {
	s := NewScheduler(nil)
	if err := RegisterContactWebhookRetryTask(s); err != nil {
		t.Fatalf("register retry task: %v", err)
	}
	task, ok := s.tasks["contact_webhook_retry"]
	if !ok {
		t.Fatal("contact webhook retry task was not registered")
	}
	if task.Schedule != "@every 1m" || task.Fn == nil {
		t.Fatalf("registered retry task = %+v", task)
	}
}
