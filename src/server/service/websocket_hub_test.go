package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/server/model"
)

func TestWebSocketHub_RegisterClient(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	userID := 1
	client := &WebSocketClient{
		ID: ClientIDForUser(userID),
		// Mock connection
		Conn:     nil,
		Hub:      hub,
		Send:     make(chan []byte, 256),
		UserID:   &userID,
		LastPing: time.Now(),
	}

	hub.RegisterClient(client)

	// Give goroutine time to process
	time.Sleep(50 * time.Millisecond)

	if !hub.IsUserConnected(userID) {
		t.Error("User should be connected after registration")
	}

	count := hub.GetConnectedCount()
	if count != 1 {
		t.Errorf("Connection count = %d, want 1", count)
	}
}

func TestWebSocketHub_UnregisterClient(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	userID := 1
	client := &WebSocketClient{
		ID:       ClientIDForUser(userID),
		Conn:     nil,
		Hub:      hub,
		Send:     make(chan []byte, 256),
		UserID:   &userID,
		LastPing: time.Now(),
	}

	hub.RegisterClient(client)
	time.Sleep(50 * time.Millisecond)

	hub.UnregisterClient(client)
	time.Sleep(50 * time.Millisecond)

	if hub.IsUserConnected(userID) {
		t.Error("User should not be connected after unregistration")
	}

	count := hub.GetConnectedCount()
	if count != 0 {
		t.Errorf("Connection count = %d, want 0", count)
	}
}

func TestWebSocketHub_IsUserConnected(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	// User not connected
	if hub.IsUserConnected(1) {
		t.Error("User 1 should not be connected initially")
	}

	// Register user
	userID := 1
	client := &WebSocketClient{
		ID:       ClientIDForUser(userID),
		Conn:     nil,
		Hub:      hub,
		Send:     make(chan []byte, 256),
		UserID:   &userID,
		LastPing: time.Now(),
	}

	hub.RegisterClient(client)
	time.Sleep(50 * time.Millisecond)

	if !hub.IsUserConnected(1) {
		t.Error("User 1 should be connected after registration")
	}

	// Different user should not be connected
	if hub.IsUserConnected(2) {
		t.Error("User 2 should not be connected")
	}
}

func TestWebSocketHub_IsAdminConnected(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	// Admin not connected
	if hub.IsAdminConnected(1) {
		t.Error("Admin 1 should not be connected initially")
	}

	// Register admin
	adminID := 1
	client := &WebSocketClient{
		ID:       ClientIDForAdmin(adminID),
		Conn:     nil,
		Hub:      hub,
		Send:     make(chan []byte, 256),
		AdminID:  &adminID,
		LastPing: time.Now(),
	}

	hub.RegisterClient(client)
	time.Sleep(50 * time.Millisecond)

	if !hub.IsAdminConnected(1) {
		t.Error("Admin 1 should be connected after registration")
	}
}

func TestWebSocketHub_BroadcastToUser(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	userID := 1
	sendChan := make(chan []byte, 256)
	client := &WebSocketClient{
		ID:       ClientIDForUser(userID),
		Conn:     nil,
		Hub:      hub,
		Send:     sendChan,
		UserID:   &userID,
		LastPing: time.Now(),
	}

	hub.RegisterClient(client)
	time.Sleep(50 * time.Millisecond)

	// Create a test notification
	notification := &model.Notification{
		ID:      "test-ulid-123",
		UserID:  &userID,
		Type:    model.NotificationTypeSuccess,
		Display: model.NotificationDisplayToast,
		Title:   "Test",
		Message: "Test message",
	}

	// Broadcast to user
	hub.BroadcastToUser(userID, notification)

	// Check if message was sent
	select {
	case msg := <-sendChan:
		var wsMsg WebSocketMessage
		err := json.Unmarshal(msg, &wsMsg)
		if err != nil {
			t.Fatalf("Failed to unmarshal message: %v", err)
		}

		if wsMsg.Type != "notification" {
			t.Errorf("Message type = %v, want 'notification'", wsMsg.Type)
		}

		// Verify notification data
		notifData, ok := wsMsg.Data.(map[string]interface{})
		if !ok {
			t.Fatal("Data is not a map")
		}

		if notifData["id"] != "test-ulid-123" {
			t.Errorf("Notification ID = %v, want 'test-ulid-123'", notifData["id"])
		}
		if notifData["title"] != "Test" {
			t.Errorf("Notification title = %v, want 'Test'", notifData["title"])
		}

	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for broadcast message")
	}
}

func TestWebSocketHub_BroadcastToAdmin(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	adminID := 1
	sendChan := make(chan []byte, 256)
	client := &WebSocketClient{
		ID:       ClientIDForAdmin(adminID),
		Conn:     nil,
		Hub:      hub,
		Send:     sendChan,
		AdminID:  &adminID,
		LastPing: time.Now(),
	}

	hub.RegisterClient(client)
	time.Sleep(50 * time.Millisecond)

	// Create a test notification
	notification := &model.Notification{
		ID:      "test-ulid-456",
		AdminID: &adminID,
		Type:    model.NotificationTypeInfo,
		Display: model.NotificationDisplayBanner,
		Title:   "Admin Test",
		Message: "Admin message",
	}

	// Broadcast to admin
	hub.BroadcastToAdmin(adminID, notification)

	// Check if message was sent
	select {
	case msg := <-sendChan:
		var wsMsg WebSocketMessage
		err := json.Unmarshal(msg, &wsMsg)
		if err != nil {
			t.Fatalf("Failed to unmarshal message: %v", err)
		}

		if wsMsg.Type != "notification" {
			t.Errorf("Message type = %v, want 'notification'", wsMsg.Type)
		}

	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for broadcast message")
	}
}

func TestWebSocketHub_GetConnectionCount(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	initialCount := hub.GetConnectedCount()
	if initialCount != 0 {
		t.Errorf("Initial count = %d, want 0", initialCount)
	}

	// Register 3 clients
	for i := 1; i <= 3; i++ {
		userID := i
		client := &WebSocketClient{
			ID:       ClientIDForUser(userID),
			Conn:     nil,
			Hub:      hub,
			Send:     make(chan []byte, 256),
			UserID:   &userID,
			LastPing: time.Now(),
		}
		hub.RegisterClient(client)
	}

	time.Sleep(100 * time.Millisecond)

	count := hub.GetConnectedCount()
	if count != 3 {
		t.Errorf("After registering 3 clients, count = %d, want 3", count)
	}
}

func TestWebSocketHub_MultipleUsersAndAdmins(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	// Register 2 users
	for i := 1; i <= 2; i++ {
		userID := i
		client := &WebSocketClient{
			ID:       ClientIDForUser(userID),
			Conn:     nil,
			Hub:      hub,
			Send:     make(chan []byte, 256),
			UserID:   &userID,
			LastPing: time.Now(),
		}
		hub.RegisterClient(client)
	}

	// Register 2 admins
	for i := 1; i <= 2; i++ {
		adminID := i
		client := &WebSocketClient{
			ID:       ClientIDForAdmin(adminID),
			Conn:     nil,
			Hub:      hub,
			Send:     make(chan []byte, 256),
			AdminID:  &adminID,
			LastPing: time.Now(),
		}
		hub.RegisterClient(client)
	}

	time.Sleep(100 * time.Millisecond)

	// Check connection count (2 users + 2 admins = 4)
	count := hub.GetConnectedCount()
	if count != 4 {
		t.Errorf("Total connections = %d, want 4", count)
	}

	// Check specific connections
	if !hub.IsUserConnected(1) {
		t.Error("User 1 should be connected")
	}
	if !hub.IsUserConnected(2) {
		t.Error("User 2 should be connected")
	}
	if !hub.IsAdminConnected(1) {
		t.Error("Admin 1 should be connected")
	}
	if !hub.IsAdminConnected(2) {
		t.Error("Admin 2 should be connected")
	}
}

func TestClientIDForUser(t *testing.T) {
	id := ClientIDForUser(123)
	expected := "user-123"
	if id != expected {
		t.Errorf("ClientIDForUser(123) = %v, want %v", id, expected)
	}
}

func TestClientIDForAdmin(t *testing.T) {
	id := ClientIDForAdmin(456)
	expected := "admin-456"
	if id != expected {
		t.Errorf("ClientIDForAdmin(456) = %v, want %v", id, expected)
	}
}

func TestWebSocketHub_Stop(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()

	// Register a client
	userID := 1
	client := &WebSocketClient{
		ID:       ClientIDForUser(userID),
		Conn:     nil,
		Hub:      hub,
		Send:     make(chan []byte, 256),
		UserID:   &userID,
		LastPing: time.Now(),
	}

	hub.RegisterClient(client)
	time.Sleep(50 * time.Millisecond)

	// Stop the hub
	hub.Stop()

	// Verify hub is stopped (this should not panic or hang)
	// The hub should gracefully handle being stopped

	time.Sleep(100 * time.Millisecond)
	// If we reach here without hanging, the test passes
}

func TestWebSocketHub_BroadcastToNonExistentUser(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	// Create a test notification
	// User that doesn't exist
	userID := 999
	notification := &model.Notification{
		ID:      "test-ulid-789",
		UserID:  &userID,
		Type:    model.NotificationTypeSuccess,
		Display: model.NotificationDisplayToast,
		Title:   "Test",
		Message: "Test message",
	}

	// This should not panic or cause errors
	hub.BroadcastToUser(999, notification)

	// Give it time to process
	time.Sleep(50 * time.Millisecond)

	// If we reach here without panic, test passes
}

func TestWebSocketMessage_Serialization(t *testing.T) {
	notification := &model.Notification{
		ID:      "test-123",
		Type:    model.NotificationTypeSuccess,
		Display: model.NotificationDisplayToast,
		Title:   "Test",
		Message: "Test message",
	}

	msg := &WebSocketMessage{
		Type: "notification",
		Data: notification,
	}

	// Serialize
	jsonData, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal message: %v", err)
	}

	// Deserialize
	var decoded WebSocketMessage
	err = json.Unmarshal(jsonData, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}

	if decoded.Type != "notification" {
		t.Errorf("Type = %v, want 'notification'", decoded.Type)
	}

	// Data should be preserved (as map[string]interface{})
	if decoded.Data == nil {
		t.Error("Data should not be nil after unmarshaling")
	}
}
