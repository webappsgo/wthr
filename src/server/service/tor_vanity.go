package service

import (
	"context"
	"crypto/ed25519"
	"encoding/base32"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/sha3"
)

// VanityGenerationStatus tracks the status of vanity address generation
type VanityGenerationStatus struct {
	Running       bool      `json:"running"`
	Prefix        string    `json:"prefix"`
	StartTime     time.Time `json:"start_time"`
	Attempts      uint64    `json:"attempts"`
	EstimatedTime string    `json:"estimated_time"`
	Address       string    `json:"address,omitempty"`
	// Never expose in JSON
	PrivateKey []byte `json:"-"`
	PublicKey  []byte `json:"-"`
}

// VanityGenerator handles vanity .onion address generation
type VanityGenerator struct {
	mu     sync.RWMutex
	status *VanityGenerationStatus
	cancel context.CancelFunc
	done   chan struct{}
	// Channel to notify when generation completes
	notifyCh chan string
}

// NewVanityGenerator creates a new vanity generator
func NewVanityGenerator() *VanityGenerator {
	return &VanityGenerator{
		done:     make(chan struct{}),
		notifyCh: make(chan string, 1),
	}
}

// Start begins generating a vanity address with the given prefix
// Prefix must be 1-6 characters, valid base32 chars only (a-z, 2-7)
func (vg *VanityGenerator) Start(prefix string) error {
	vg.mu.Lock()
	defer vg.mu.Unlock()

	// Validate prefix
	prefix = strings.ToLower(prefix)
	if len(prefix) < 1 || len(prefix) > 6 {
		return fmt.Errorf("prefix must be 1-6 characters, got %d", len(prefix))
	}

	// Validate characters (base32: a-z, 2-7)
	for _, c := range prefix {
		if !((c >= 'a' && c <= 'z') || (c >= '2' && c <= '7')) {
			return fmt.Errorf("invalid character in prefix: %c (use only a-z, 2-7)", c)
		}
	}

	// Check if already running
	if vg.status != nil && vg.status.Running {
		return fmt.Errorf("vanity generation already in progress for prefix: %s", vg.status.Prefix)
	}

	// Initialize status
	vg.status = &VanityGenerationStatus{
		Running:   true,
		Prefix:    prefix,
		StartTime: time.Now(),
		Attempts:  0,
	}

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	vg.cancel = cancel

	// Start generation in background
	go vg.generate(ctx, prefix)

	log.Printf("INFO: Started vanity address generation for prefix: %s", prefix)
	return nil
}

// generate runs the actual generation algorithm
func (vg *VanityGenerator) generate(ctx context.Context, prefix string) {
	defer close(vg.done)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			vg.mu.Lock()
			vg.status.Running = false
			vg.mu.Unlock()
			log.Printf("INFO: Vanity generation cancelled for prefix: %s (attempts: %d)", prefix, vg.status.Attempts)
			return

		case <-ticker.C:
			// Update estimated time every second
			vg.updateEstimate()

		default:
			// Try to generate a matching key
			if vg.tryGenerate(prefix) {
				vg.mu.Lock()
				vg.status.Running = false
				vg.mu.Unlock()

				// Notify completion
				select {
				case vg.notifyCh <- vg.status.Address:
				default:
				}

				log.Printf("OK: Found vanity address: %s (attempts: %d, time: %v)",
					vg.status.Address, vg.status.Attempts, time.Since(vg.status.StartTime))
				return
			}
		}
	}
}

// tryGenerate attempts to generate one key pair and check if it matches
func (vg *VanityGenerator) tryGenerate(prefix string) bool {
	// Generate ed25519 key pair
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		log.Printf("WARNING: Failed to generate key: %v", err)
		return false
	}

	// Convert to v3 onion address
	address := publicKeyToOnionAddress(publicKey)

	// Increment attempt counter
	vg.mu.Lock()
	vg.status.Attempts++
	vg.mu.Unlock()

	// Check if it matches the prefix
	if strings.HasPrefix(address, prefix) {
		vg.mu.Lock()
		vg.status.Address = address + ".onion"
		vg.status.PrivateKey = privateKey
		vg.status.PublicKey = publicKey
		vg.mu.Unlock()
		return true
	}

	return false
}

// publicKeyToOnionAddress converts an ed25519 public key to a v3 onion address
func publicKeyToOnionAddress(publicKey ed25519.PublicKey) string {
	// v3 onion address format:
	// onion_address = base32(PUBKEY | CHECKSUM | VERSION) + ".onion"
	// CHECKSUM = H(".onion checksum" | PUBKEY | VERSION)[:2]
	// VERSION = 0x03

	const version = byte(0x03)
	checksumConst := []byte(".onion checksum")

	// Calculate checksum
	h := sha3.New256()
	h.Write(checksumConst)
	h.Write(publicKey)
	h.Write([]byte{version})
	checksum := h.Sum(nil)[:2]

	// Construct address bytes: PUBKEY (32) | CHECKSUM (2) | VERSION (1)
	addressBytes := make([]byte, 0, 35)
	addressBytes = append(addressBytes, publicKey...)
	addressBytes = append(addressBytes, checksum...)
	addressBytes = append(addressBytes, version)

	// Encode to base32 (without padding)
	address := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(addressBytes)

	return strings.ToLower(address)
}

// updateEstimate calculates estimated time remaining based on attempts/sec
func (vg *VanityGenerator) updateEstimate() {
	vg.mu.Lock()
	defer vg.mu.Unlock()

	if !vg.status.Running {
		return
	}

	elapsed := time.Since(vg.status.StartTime).Seconds()
	if elapsed < 1 {
		return
	}

	attemptsPerSec := float64(vg.status.Attempts) / elapsed
	if attemptsPerSec < 1 {
		vg.status.EstimatedTime = "calculating..."
		return
	}

	// Estimate based on probability
	// For base32 (32 chars), each character position has 1/32 probability
	// For prefix length N, expected attempts = 32^N
	prefixLen := len(vg.status.Prefix)
	expectedAttempts := 1.0
	for i := 0; i < prefixLen; i++ {
		expectedAttempts *= 32
	}

	remainingAttempts := expectedAttempts - float64(vg.status.Attempts)
	if remainingAttempts < 0 {
		// At least 10% more
		remainingAttempts = expectedAttempts * 0.1
	}

	estimatedSeconds := remainingAttempts / attemptsPerSec
	vg.status.EstimatedTime = formatDuration(time.Duration(estimatedSeconds) * time.Second)
}

// formatDuration formats a duration in human-readable form
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours %d minutes", int(d.Hours()), int(d.Minutes())%60)
	}
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%d days %d hours", days, hours)
}

// Cancel stops the vanity generation
func (vg *VanityGenerator) Cancel() error {
	vg.mu.Lock()
	defer vg.mu.Unlock()

	if vg.status == nil || !vg.status.Running {
		return fmt.Errorf("no vanity generation in progress")
	}

	if vg.cancel != nil {
		vg.cancel()
	}

	return nil
}

// GetStatus returns the current generation status
func (vg *VanityGenerator) GetStatus() *VanityGenerationStatus {
	vg.mu.RLock()
	defer vg.mu.RUnlock()

	if vg.status == nil {
		return nil
	}

	// Return a copy to avoid race conditions
	statusCopy := *vg.status
	return &statusCopy
}

// Wait waits for generation to complete or be cancelled
func (vg *VanityGenerator) Wait() {
	<-vg.done
}

// GetNotificationChannel returns a channel that receives the address when generation completes
func (vg *VanityGenerator) GetNotificationChannel() <-chan string {
	return vg.notifyCh
}

// GetKeys returns the generated keys (only if generation completed successfully)
func (vg *VanityGenerator) GetKeys() (publicKey, privateKey []byte, err error) {
	vg.mu.RLock()
	defer vg.mu.RUnlock()

	if vg.status == nil {
		return nil, nil, fmt.Errorf("no generation has been performed")
	}

	if vg.status.Running {
		return nil, nil, fmt.Errorf("generation still in progress")
	}

	if vg.status.Address == "" {
		return nil, nil, fmt.Errorf("generation did not complete successfully")
	}

	return vg.status.PublicKey, vg.status.PrivateKey, nil
}

// EstimateTime provides time estimates for different prefix lengths
func EstimateTime(prefixLength int, attemptsPerSec float64) string {
	if attemptsPerSec == 0 {
		// Default estimate: 100k attempts/sec
		attemptsPerSec = 100000
	}

	expectedAttempts := 1.0
	for i := 0; i < prefixLength; i++ {
		expectedAttempts *= 32
	}

	estimatedSeconds := expectedAttempts / attemptsPerSec
	return formatDuration(time.Duration(estimatedSeconds) * time.Second)
}
