package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/webappsgo/wthr/src/config"
)

const (
	// simpleCaptchaTTL is how long a built-in challenge stays valid (AI.md PART 16)
	simpleCaptchaTTL = 10 * time.Minute
	// simpleCaptchaMax bounds memory: the oldest challenges are dropped first
	simpleCaptchaMax = 5000
	// providerTimeout bounds a recaptcha/hcaptcha verification call
	providerTimeout = 10 * time.Second
)

// ErrCaptchaFailed means the supplied captcha answer or token was not accepted.
var ErrCaptchaFailed = errors.New("captcha verification failed")

// ErrCaptchaUnavailable means the provider could not be reached; the submission is
// rejected with a retryable error and never accepted unverified (fail closed).
var ErrCaptchaUnavailable = errors.New("captcha provider unavailable")

type simpleChallenge struct {
	answerHash [32]byte
	expires    time.Time
}

// SimpleCaptcha is the built-in, self-hosted challenge: no third party, no keys, works
// over Tor and with JavaScript disabled. Only the hashed answer is stored; each
// challenge is single-use and expires after ten minutes.
type SimpleCaptcha struct {
	mu         sync.Mutex
	challenges map[string]simpleChallenge
	order      []string
}

// NewSimpleCaptcha creates an empty built-in captcha store.
func NewSimpleCaptcha() *SimpleCaptcha {
	return &SimpleCaptcha{challenges: make(map[string]simpleChallenge)}
}

func captchaDigest(answer string) [32]byte {
	return sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(answer))))
}

func randomInt(n int64) int64 {
	v, err := rand.Int(rand.Reader, big.NewInt(n))
	if err != nil {
		return 0
	}
	return v.Int64()
}

// New issues a challenge. a and b are the two operands of the addition the visitor
// must solve; the caller renders them through a translated sentence.
func (c *SimpleCaptcha) New() (id string, a, b int) {
	a, b = int(randomInt(9))+1, int(randomInt(9))+1
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", a, b
	}
	id = hex.EncodeToString(raw)

	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	c.pruneLocked(now)
	c.challenges[id] = simpleChallenge{answerHash: captchaDigest(strconv.Itoa(a + b)), expires: now.Add(simpleCaptchaTTL)}
	c.order = append(c.order, id)
	for len(c.order) > simpleCaptchaMax {
		delete(c.challenges, c.order[0])
		c.order = c.order[1:]
	}
	return id, a, b
}

func (c *SimpleCaptcha) pruneLocked(now time.Time) {
	kept := c.order[:0]
	for _, id := range c.order {
		ch, ok := c.challenges[id]
		if !ok {
			continue
		}
		if now.After(ch.expires) {
			delete(c.challenges, id)
			continue
		}
		kept = append(kept, id)
	}
	c.order = kept
}

// Verify consumes the challenge (single-use, whether the answer is right or not) and
// reports whether the answer was correct.
func (c *SimpleCaptcha) Verify(id, answer string) bool {
	c.mu.Lock()
	ch, ok := c.challenges[id]
	delete(c.challenges, id)
	c.mu.Unlock()
	if !ok || time.Now().After(ch.expires) {
		return false
	}
	got := captchaDigest(answer)
	return subtle.ConstantTimeCompare(got[:], ch.answerHash[:]) == 1
}

// providerEndpoint maps a provider name to its verification URL.
func providerEndpoint(provider string) string {
	switch provider {
	case config.CaptchaReCaptcha:
		return "https://www.google.com/recaptcha/api/siteverify"
	case config.CaptchaHCaptcha:
		return "https://api.hcaptcha.com/siteverify"
	}
	return ""
}

// VerifyProvider checks a recaptcha or hcaptcha response token and accepts only
// "success": true. The secret is sent server-side only and never logged.
func VerifyProvider(ctx context.Context, client *http.Client, provider, secret, response, remoteIP string) error {
	return verifyProviderAt(ctx, client, providerEndpoint(provider), provider, secret, response, remoteIP)
}

func verifyProviderAt(ctx context.Context, client *http.Client, endpoint, provider, secret, response, remoteIP string) error {
	if endpoint == "" {
		return fmt.Errorf("unknown captcha provider %q", provider)
	}
	if strings.TrimSpace(response) == "" {
		return ErrCaptchaFailed
	}
	if client == nil {
		client = &http.Client{Timeout: providerTimeout}
	}
	form := url.Values{"secret": {secret}, "response": {response}}
	if provider == config.CaptchaReCaptcha && remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	ctx, cancel := context.WithTimeout(ctx, providerTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return ErrCaptchaUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return ErrCaptchaUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ErrCaptchaUnavailable
	}
	var body struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&body); err != nil {
		return ErrCaptchaUnavailable
	}
	if !body.Success {
		return ErrCaptchaFailed
	}
	return nil
}
