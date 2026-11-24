package handler

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const contactAntispamBodyLimit = 256 << 10

var contactMarkupPattern = regexp.MustCompile(`(?i)(</?[a-z][^>]*|<!|\[(?:/?(?:url|img|b))(?:\s|=|\])|&(?:lt|gt|#0*60|#x0*3c|#0*62|#x0*3e);)`)
var contactLinkPattern = regexp.MustCompile(`(?i)(?:\b[a-z][a-z0-9+.-]*://[^\s<>]+|\bwww\.[^\s<>]+|\b(?:[a-z0-9-]+\.)+[a-z]{2,}(?:/[^\s<>]*)?)`)
var contactDirectionalPattern = regexp.MustCompile(`[\x{200B}-\x{200F}\x{202A}-\x{202E}\x{2060}\x{FEFF}]`)

// ContactAntispamConfig holds parsed server.pages.contact.antispam values.
type ContactAntispamConfig struct {
	MinFill, MaxAge, Window, BanDuration, ReviewMin, ReviewMax, DuplicateFor time.Duration
	PerIP, PerEmail, Global, MaxLinks, Strikes                               int
	RejectMarkup                                                             bool
	BlockedWords, BlockedEmailDomains                                        []string
}

// ContactAntispamValues carries the complete form snapshot with absent optional fields as empty strings.
type ContactAntispamValues map[string]string

// ContactAntispam provides bounded per-process replay, rate, strike and duplicate state.
type ContactAntispam struct {
	mu         sync.Mutex
	secret     []byte
	cfg        ContactAntispamConfig
	replays    map[string]time.Time
	counts     map[string][]time.Time
	strikes    map[string][]time.Time
	bans       map[string]time.Time
	duplicates map[string]time.Time
}

func NewContactAntispam(secret []byte, cfg ContactAntispamConfig) *ContactAntispam {
	return &ContactAntispam{secret: append([]byte(nil), secret...), cfg: cfg, replays: map[string]time.Time{}, counts: map[string][]time.Time{}, strikes: map[string][]time.Time{}, bans: map[string]time.Time{}, duplicates: map[string]time.Time{}}
}

// SetConfig applies hot-reloaded settings without clearing abuse state.
func (a *ContactAntispam) SetConfig(cfg ContactAntispamConfig) {
	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
}

// NewContactFormToken creates a stateless, IP-independent, signed render token.
func (a *ContactAntispam) NewContactFormToken(now time.Time) (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	i, n := strconv.FormatInt(now.Unix(), 10), hex.EncodeToString(nonce[:])
	return i + "." + n + "." + a.mac("contact-form|"+i+"|"+n), nil
}

// ConsumeContactFormToken verifies expiry and records the nonce exactly once.
func (a *ContactAntispam) ConsumeContactFormToken(token string, now time.Time) (bool, string) {
	p := strings.Split(token, ".")
	if len(p) != 3 || len(p[1]) != 32 {
		return false, "fill_time"
	}
	i, err := strconv.ParseInt(p[0], 10, 64)
	if err != nil {
		return false, "fill_time"
	}
	issued := time.Unix(i, 0)
	age := now.Sub(issued)
	if age < a.cfg.MinFill || age < 0 || (a.cfg.MaxAge > 0 && age > a.cfg.MaxAge) {
		return false, "fill_time"
	}
	if !hmac.Equal([]byte(p[2]), []byte(a.mac("contact-form|"+p[0]+"|"+p[1]))) {
		return false, "fill_time"
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneLocked(now)
	if _, ok := a.replays[p[1]]; ok {
		return false, "replay"
	}
	if len(a.replays) >= 50000 {
		a.evictOldestReplayLocked()
	}
	expires := issued.Add(a.cfg.MaxAge)
	if a.cfg.MaxAge <= 0 {
		expires = now.Add(24 * time.Hour)
	}
	a.replays[p[1]] = expires
	return true, ""
}

// NewContactReviewToken signs a normalized form snapshot. drop marks the honeypot path.
func (a *ContactAntispam) NewContactReviewToken(values ContactAntispamValues, now time.Time, drop bool) (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	i, n, flag := strconv.FormatInt(now.Unix(), 10), hex.EncodeToString(nonce[:]), "-"
	if drop {
		flag = "d"
	}
	message := "contact-review|" + i + "|" + n + "|" + flag + "|" + contactValuesHash(values)
	return i + "." + n + "." + flag + "." + a.mac(message), nil
}

// VerifyContactReviewToken validates the age and immutable value snapshot and returns the drop flag.
func (a *ContactAntispam) VerifyContactReviewToken(token string, values ContactAntispamValues, now time.Time) (bool, bool) {
	return a.checkContactReviewToken(token, values, now, false)
}

// ConsumeContactReviewToken validates a confirmation and records its nonce once.
func (a *ContactAntispam) ConsumeContactReviewToken(token string, values ContactAntispamValues, now time.Time) (bool, bool) {
	return a.checkContactReviewToken(token, values, now, true)
}

func (a *ContactAntispam) checkContactReviewToken(token string, values ContactAntispamValues, now time.Time, consume bool) (bool, bool) {
	p := strings.Split(token, ".")
	if len(p) != 4 || (p[2] != "-" && p[2] != "d") {
		return false, false
	}
	i, err := strconv.ParseInt(p[0], 10, 64)
	if err != nil {
		return false, false
	}
	age := now.Sub(time.Unix(i, 0))
	if age < a.cfg.ReviewMin || age < 0 || (a.cfg.ReviewMax > 0 && age > a.cfg.ReviewMax) {
		return false, false
	}
	message := "contact-review|" + p[0] + "|" + p[1] + "|" + p[2] + "|" + contactValuesHash(values)
	if !hmac.Equal([]byte(p[3]), []byte(a.mac(message))) {
		return false, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneLocked(now)
	key := "review:" + p[1]
	if _, used := a.replays[key]; used {
		return false, false
	}
	if consume {
		if len(a.replays) >= 50000 {
			a.evictOldestReplayLocked()
		}
		expires := time.Unix(i, 0).Add(a.cfg.ReviewMax)
		if a.cfg.ReviewMax <= 0 {
			expires = now.Add(24 * time.Hour)
		}
		a.replays[key] = expires
	}
	return true, p[2] == "d"
}

// CheckContactOrigin requires a browser form request to identify this site's own origin.
func CheckContactOrigin(r *http.Request, siteOrigin string) bool {
	fetch := strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))
	if fetch != "" && fetch != "same-origin" {
		return false
	}
	provided := strings.TrimSpace(r.Header.Get("Origin"))
	originHeader := provided != ""
	if provided == "" {
		provided = strings.TrimSpace(r.Header.Get("Referer"))
	}
	if provided == "" {
		return fetch == "same-origin"
	}
	got, e1 := url.Parse(provided)
	want, e2 := url.Parse(siteOrigin)
	if e1 != nil || e2 != nil || got.User != nil || want.User != nil || got.Hostname() == "" || want.Hostname() == "" || !strings.EqualFold(got.Scheme, want.Scheme) {
		return false
	}
	if originHeader && (got.Path != "" || got.RawQuery != "" || got.Fragment != "") {
		return false
	}
	port := func(u *url.URL) string {
		if value := u.Port(); value != "" {
			return value
		}
		if strings.EqualFold(u.Scheme, "https") {
			return "443"
		}
		if strings.EqualFold(u.Scheme, "http") {
			return "80"
		}
		return ""
	}
	return strings.EqualFold(strings.TrimSuffix(got.Hostname(), "."), strings.TrimSuffix(want.Hostname(), ".")) && port(got) == port(want)
}

// ValidateContactShape checks form field names, required values, UTF-8, controls and browser origin.
func ValidateContactShape(r *http.Request, values ContactAntispamValues, allowed, required []string, htmlForm bool, siteOrigin string) string {
	if r.ContentLength > contactAntispamBodyLimit {
		return "shape"
	}
	for key := range values {
		if !containsContactString(allowed, key) {
			return "shape"
		}
	}
	for _, key := range required {
		if _, ok := values[key]; !ok {
			return "shape"
		}
	}
	for key, value := range values {
		if !utf8.ValidString(key) || !utf8.ValidString(value) || contactDirectionalPattern.MatchString(value) {
			return "shape"
		}
		if key == "name" || key == "email" || key == "subject" {
			for _, char := range value {
				if unicode.IsControl(char) {
					return "shape"
				}
			}
		}
	}
	if email := values["email"]; email != "" && (strings.ContainsAny(email, ",;<>\r\n") || strings.Count(email, "@") != 1) {
		return "shape"
	}
	if htmlForm && !CheckContactOrigin(r, siteOrigin) {
		return "origin"
	}
	return ""
}

// CheckContactContent enforces markup, link host/count, blocked word, and email-domain rules.
func (a *ContactAntispam) CheckContactContent(values ContactAntispamValues, acceptedHosts []string, security bool) string {
	if security {
		return ""
	}
	for key, value := range values {
		if a.cfg.RejectMarkup && (contactMarkupPattern.MatchString(value) || contactMarkupPattern.MatchString(html.UnescapeString(value))) {
			return "markup"
		}
		if key == "email" {
			continue
		}
	}
	linksSeen := 0
	for key, value := range values {
		// Email domains are contact addresses, not message links.
		if key == "email" {
			continue
		}
		links := contactLinkPattern.FindAllString(value, -1)
		if key == "name" || key == "subject" {
			if len(links) > 0 {
				return "content"
			}
		}
		for _, link := range links {
			linksSeen++
			host := contactLinkHost(link)
			if host == "" || !contactHostAllowed(host, acceptedHosts) {
				return "content"
			}
		}
	}
	if a.cfg.MaxLinks > 0 && linksSeen > a.cfg.MaxLinks {
		return "content"
	}
	for key, value := range values {
		if key == "type" || key == "good_faith" || key == "accuracy" {
			continue
		}
		lower := strings.ToLower(value)
		for _, blocked := range a.cfg.BlockedWords {
			if blocked != "" && strings.Contains(lower, strings.ToLower(blocked)) {
				return "content"
			}
		}
	}
	if at := strings.LastIndex(values["email"], "@"); at >= 0 {
		domain := strings.ToLower(strings.TrimSuffix(values["email"][at+1:], "."))
		for _, blocked := range a.cfg.BlockedEmailDomains {
			if domain == strings.ToLower(strings.TrimPrefix(blocked, "@")) {
				return "content"
			}
		}
	}
	return ""
}

// CheckRateLimit checks limits; count=false performs a non-mutating step-one preflight.
func (a *ContactAntispam) CheckRateLimit(ip, email string, now time.Time, count, overlay bool) (string, time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneLocked(now)
	if !overlay {
		if until := a.bans[ip]; until.After(now) {
			return "banned", until.Sub(now)
		}
	}
	window := a.cfg.Window
	if window <= 0 {
		return "", 0
	}
	type bucket struct {
		key, reason string
		limit       int
	}
	buckets := []bucket{}
	if !overlay {
		buckets = append(buckets, bucket{"ip:" + ip, "rate_ip", a.cfg.PerIP})
	}
	buckets = append(buckets, bucket{"email:" + strings.ToLower(strings.TrimSpace(email)), "rate_email", a.cfg.PerEmail}, bucket{"global", "rate_global", a.cfg.Global})
	for _, b := range buckets {
		if b.limit > 0 && len(contactTimesInWindow(a.counts[b.key], now, window)) >= b.limit {
			return b.reason, window
		}
	}
	if count {
		for _, b := range buckets {
			if b.limit <= 0 {
				continue
			}
			if _, exists := a.counts[b.key]; !exists && len(a.counts) >= 50000 {
				evictOldestTimes(a.counts)
			}
			a.counts[b.key] = append(contactTimesInWindow(a.counts[b.key], now, window), now)
		}
	}
	return "", 0
}

// RecordContactStrike records a configured reject event and returns any resulting ban duration.
func (a *ContactAntispam) RecordContactStrike(ip string, now time.Time) time.Duration {
	if ip == "" || a.cfg.Strikes <= 0 {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneLocked(now)
	window := a.cfg.Window
	if window <= 0 {
		return 0
	}
	items := append(contactTimesInWindow(a.strikes[ip], now, window), now)
	if len(items) >= a.cfg.Strikes && a.cfg.BanDuration > 0 {
		if _, exists := a.bans[ip]; !exists && len(a.bans) >= 50000 {
			evictOldestExpiry(a.bans)
		}
		a.bans[ip] = now.Add(a.cfg.BanDuration)
		delete(a.strikes, ip)
		return a.cfg.BanDuration
	}
	if _, exists := a.strikes[ip]; !exists && len(a.strikes) >= 50000 {
		evictOldestTimes(a.strikes)
	}
	a.strikes[ip] = items
	return 0
}

// IsContactDuplicate atomically remembers a normalized sender/subject/message digest.
func (a *ContactAntispam) IsContactDuplicate(values ContactAntispamValues, now time.Time) bool {
	if a.cfg.DuplicateFor <= 0 {
		return false
	}
	material := normalizeContactValue(values["email"]) + "\x00" + normalizeContactValue(values["subject"]) + "\x00" + normalizeContactValue(values["message"])
	sum := sha256.Sum256([]byte(material))
	key := hex.EncodeToString(sum[:])
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneLocked(now)
	if until, ok := a.duplicates[key]; ok && until.After(now) {
		return true
	}
	if len(a.duplicates) >= 10000 {
		a.evictOldestDuplicateLocked()
	}
	a.duplicates[key] = now.Add(a.cfg.DuplicateFor)
	return false
}

func (a *ContactAntispam) mac(message string) string {
	m := hmac.New(sha256.New, a.secret)
	_, _ = m.Write([]byte(message))
	return hex.EncodeToString(m.Sum(nil))
}
func contactValuesHash(values ContactAntispamValues) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		value := strings.Join(strings.Fields(strings.TrimSpace(values[k])), " ")
		if k == "email" {
			value = strings.ToLower(value)
		}
		_, _ = h.Write([]byte(k))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{0xff})
	}
	return hex.EncodeToString(h.Sum(nil))
}
func normalizeContactValue(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}
func contactLinkHost(raw string) string {
	if strings.HasPrefix(strings.ToLower(raw), "www.") {
		raw = "https://" + raw
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
}
func contactHostAllowed(host string, allowed []string) bool {
	normalize := func(value string) string {
		value = strings.TrimSpace(strings.TrimSuffix(value, "."))
		if parsed, err := url.Parse("//" + value); err == nil && parsed.Hostname() != "" {
			value = parsed.Hostname()
		}
		return strings.ToLower(strings.Trim(value, "[]"))
	}
	host = normalize(host)
	for _, candidate := range allowed {
		if host == normalize(candidate) {
			return true
		}
	}
	return false
}
func containsContactString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
func contactTimesInWindow(items []time.Time, now time.Time, window time.Duration) []time.Time {
	if window <= 0 {
		return nil
	}
	kept := items[:0]
	for _, item := range items {
		if !item.After(now) && now.Sub(item) <= window {
			kept = append(kept, item)
		}
	}
	return kept
}
func (a *ContactAntispam) pruneLocked(now time.Time) {
	for k, t := range a.replays {
		if !t.After(now) {
			delete(a.replays, k)
		}
	}
	for k, t := range a.bans {
		if !t.After(now) {
			delete(a.bans, k)
		}
	}
	for k, t := range a.duplicates {
		if !t.After(now) {
			delete(a.duplicates, k)
		}
	}
	for k, x := range a.counts {
		if kept := contactTimesInWindow(x, now, a.cfg.Window); len(kept) == 0 {
			delete(a.counts, k)
		} else {
			a.counts[k] = kept
		}
	}
	for k, x := range a.strikes {
		if kept := contactTimesInWindow(x, now, a.cfg.Window); len(kept) == 0 {
			delete(a.strikes, k)
		} else {
			a.strikes[k] = kept
		}
	}
}
func (a *ContactAntispam) evictOldestReplayLocked() {
	var key string
	var oldest time.Time
	for k, t := range a.replays {
		if oldest.IsZero() || t.Before(oldest) {
			key, oldest = k, t
		}
	}
	delete(a.replays, key)
}
func (a *ContactAntispam) evictOldestDuplicateLocked() {
	var key string
	var oldest time.Time
	for k, t := range a.duplicates {
		if oldest.IsZero() || t.Before(oldest) {
			key, oldest = k, t
		}
	}
	delete(a.duplicates, key)
}
func evictOldestExpiry(values map[string]time.Time) {
	var key string
	var oldest time.Time
	for k, t := range values {
		if oldest.IsZero() || t.Before(oldest) {
			key, oldest = k, t
		}
	}
	delete(values, key)
}
func evictOldestTimes(values map[string][]time.Time) {
	var key string
	var oldest time.Time
	for k, items := range values {
		if len(items) > 0 && (oldest.IsZero() || items[0].Before(oldest)) {
			key, oldest = k, items[0]
		}
	}
	delete(values, key)
}
