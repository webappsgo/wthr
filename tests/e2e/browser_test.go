//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

const adminPassword = "Wthr-E2E-Admin-Password-93!"

var (
	baseURL     string
	chromeURL   string
	serverCmd   *exec.Cmd
	serverLog   *os.File
	serverDir   string
	artifactDir string
)

func TestMain(m *testing.M) {
	var err error
	baseURL = strings.TrimRight(os.Getenv("E2E_BASE_URL"), "/")
	chromeURL = os.Getenv("E2E_CHROME_WS_URL")
	binary := os.Getenv("E2E_SERVER_BINARY")
	if baseURL == "" || chromeURL == "" || binary == "" {
		fmt.Fprintln(os.Stderr, "E2E_BASE_URL, E2E_CHROME_WS_URL, and E2E_SERVER_BINARY are required")
		os.Exit(2)
	}
	if err := waitForChrome(); err != nil {
		fmt.Fprintln(os.Stderr, "connect to headless Chromium:", err)
		os.Exit(1)
	}

	tempBase := filepath.Join(os.TempDir(), os.Getenv("PROJECT_ORG"))
	if err = os.MkdirAll(tempBase, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "create E2E temp root:", err)
		os.Exit(2)
	}
	serverDir, err = os.MkdirTemp(tempBase, os.Getenv("INTERNAL_NAME")+"-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create E2E server temp directory:", err)
		os.Exit(2)
	}
	artifactDir = filepath.Join(serverDir, "artifacts")
	if err = os.MkdirAll(artifactDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "create E2E artifact directory:", err)
		os.Exit(2)
	}
	if err = startServer(binary); err != nil {
		fmt.Fprintln(os.Stderr, "start E2E server:", err)
		stopServer()
		fmt.Fprintln(os.Stderr, "server logs retained at", filepath.Join(serverDir, "server.log"))
		os.Exit(1)
	}
	if err = createAdmin(); err != nil {
		fmt.Fprintln(os.Stderr, "prepare E2E admin:", err)
		stopServer()
		fmt.Fprintln(os.Stderr, "server logs retained at", filepath.Join(serverDir, "server.log"))
		os.Exit(1)
	}

	code := m.Run()
	stopServer()
	if code == 0 {
		_ = os.RemoveAll(serverDir)
	} else {
		fmt.Fprintln(os.Stderr, "E2E artifacts retained at", serverDir)
	}
	os.Exit(code)
}

func startServer(binary string) error {
	for _, dir := range []string{"config", "data", "log", "cache", "backup"} {
		if err := os.MkdirAll(filepath.Join(serverDir, dir), 0o700); err != nil {
			return fmt.Errorf("create %s directory: %w", dir, err)
		}
	}
	config := "server:\n  rate_limit:\n    enabled: false\n  tor:\n    enabled: false\n  i2p:\n    enabled: false\n  scheduler:\n    enabled: false\nusers:\n  enabled: true\n  registration:\n    mode: open\n    require_email_verification: false\n"
	if err := os.WriteFile(filepath.Join(serverDir, "config", "server.yml"), []byte(config), 0o600); err != nil {
		return fmt.Errorf("write isolated E2E config: %w", err)
	}
	var err error
	serverLog, err = os.OpenFile(filepath.Join(serverDir, "server.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open server log: %w", err)
	}
	port := os.Getenv("E2E_SERVER_PORT")
	if port == "" {
		port = "64127"
	}
	serverCmd = exec.Command(binary,
		"--mode", "development",
		"--config", filepath.Join(serverDir, "config"),
		"--data", filepath.Join(serverDir, "data"),
		"--log", filepath.Join(serverDir, "log"),
		"--cache", filepath.Join(serverDir, "cache"),
		"--backup", filepath.Join(serverDir, "backup"),
		"--address", "0.0.0.0",
		"--port", port,
		"--debug",
	)
	serverCmd.Stdout = serverLog
	serverCmd.Stderr = serverLog
	if err := serverCmd.Start(); err != nil {
		return fmt.Errorf("launch server process: %w", err)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(baseURL + "/server/healthz")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		if err := serverCmd.Process.Signal(syscall.Signal(0)); err != nil {
			return fmt.Errorf("server exited before becoming ready: %w", err)
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("server did not answer health checks before the 60 second deadline")
}

func waitForChrome() error {
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(ctx, chromeURL)
		browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
		err := chromedp.Run(browserCtx, chromedp.Navigate("about:blank"))
		cancelBrowser()
		cancelAlloc()
		cancel()
		if err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("browser did not answer within 60 seconds")
}

func createAdmin() error {
	setupTokenPattern := regexp.MustCompile(`Setup Token: ([A-Za-z0-9]{16,})`)
	deadline := time.Now().Add(30 * time.Second)
	var setupToken string
	for time.Now().Before(deadline) {
		logData, err := os.ReadFile(filepath.Join(serverDir, "server.log"))
		if err == nil {
			if match := setupTokenPattern.FindSubmatch(logData); len(match) == 2 {
				setupToken = string(match[1])
				break
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	if setupToken == "" {
		return fmt.Errorf("setup token was not written to the server log")
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("create setup cookie jar: %w", err)
	}
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second}
	if response, err := client.PostForm(baseURL+"/server/administration/verify-token", url.Values{"setup_token": {setupToken}}); err != nil {
		return fmt.Errorf("verify setup token: %w", err)
	} else {
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if response.StatusCode >= 400 {
			return fmt.Errorf("verify setup token returned HTTP %d", response.StatusCode)
		}
	}
	form := url.Values{
		"username":         {"e2eadmin"},
		"email":            {"admin@wthr.test"},
		"password":         {adminPassword},
		"confirm_password": {adminPassword},
	}
	request, err := http.NewRequest(http.MethodPost, baseURL+"/server/administration/setup", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create setup request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("create E2E admin account: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("create E2E admin returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func stopServer() {
	if serverCmd != nil && serverCmd.Process != nil {
		_ = serverCmd.Process.Signal(os.Interrupt)
		wait := make(chan struct{})
		go func() {
			_ = serverCmd.Wait()
			close(wait)
		}()
		select {
		case <-wait:
		case <-time.After(10 * time.Second):
			_ = serverCmd.Process.Kill()
			<-wait
		}
	}
	if serverLog != nil {
		_ = serverLog.Close()
	}
}

func newBrowser(t *testing.T, noJavaScript bool) context.Context {
	t.Helper()
	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(context.Background(), chromeURL)
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	t.Cleanup(cancelBrowser)
	t.Cleanup(cancelAlloc)
	if noJavaScript {
		if err := chromedp.Run(browserCtx, emulation.SetScriptExecutionDisabled(true)); err != nil {
			t.Fatalf("disable JavaScript for no-JS tier: %v", err)
		}
	}
	return browserCtx
}

func saveBrowserArtifacts(t *testing.T, ctx context.Context, label string) {
	t.Helper()
	name := strings.NewReplacer("/", "_", "?", "_", "&", "_", "=", "_").Replace(label)
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		var screenshot []byte
		if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&screenshot)); err == nil {
			_ = os.WriteFile(filepath.Join(artifactDir, name+".png"), screenshot, 0o600)
		}
		var html string
		if err := chromedp.Run(ctx, chromedp.OuterHTML("html", &html)); err == nil {
			_ = os.WriteFile(filepath.Join(artifactDir, name+".html"), []byte(html), 0o600)
		}
	})
}

func TestSSRWeatherAndAccountPages(t *testing.T) {
	pages := []struct {
		path string
		text string
	}{
		{"/", "weather"},
		{"/server/about", "about"},
		{"/server/help", "help"},
		{"/server/auth/login", "login"},
		{"/moon?location=51.5074,-0.1278", "moon"},
		{"/severe-weather?location=40.7128,-74.0060", "severe"},
	}
	client := &http.Client{Timeout: 20 * time.Second}
	for _, page := range pages {
		page := page
		t.Run(page.path, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, baseURL+page.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Accept", "text/html")
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusOK {
				t.Fatalf("GET %s returned HTTP %d: %s", page.path, response.StatusCode, string(body[:min(len(body), 1200)]))
			}
			if !strings.Contains(response.Header.Get("Content-Type"), "text/html") {
				t.Fatalf("GET %s content type = %q, want text/html", page.path, response.Header.Get("Content-Type"))
			}
			for _, marker := range []string{"<title>", `lang="en"`, `name="viewport"`, "<main"} {
				if !strings.Contains(string(body), marker) {
					t.Errorf("GET %s SSR response is missing %q", page.path, marker)
				}
			}
			if strings.Contains(strings.ToLower(string(body)), "<title></title>") {
				t.Errorf("GET %s has an empty title", page.path)
			}
			if !strings.Contains(strings.ToLower(string(body)), page.text) {
				t.Errorf("GET %s does not include page-specific %q content", page.path, page.text)
			}
		})
	}
}

func TestSSRUnknownRouteIsThemedNotFound(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, baseURL+"/e2e-not-a-route", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Accept", "text/html")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown route returned HTTP %d, want 404", response.StatusCode)
	}
	if !strings.Contains(string(body), "<main") || strings.Contains(string(body), "goroutine ") {
		t.Fatalf("unknown route did not render the themed error page")
	}
}

func TestNoJavaScriptLoginForm(t *testing.T) {
	ctx := newBrowser(t, true)
	saveBrowserArtifacts(t, ctx, "nojs-login")
	var form struct {
		Action     string `json:"action"`
		Method     string `json:"method"`
		CSRF       bool   `json:"csrf"`
		LoginField bool   `json:"loginField"`
	}
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/server/auth/login"),
		chromedp.Evaluate(`(() => { const f = document.querySelector("form"); return {action:f?.getAttribute("action"), method:f?.getAttribute("method"), csrf:!!f?.querySelector("input[name=csrf_token]"), loginField:!!f?.querySelector("input[name=identifier]")}; })()`, &form),
	); err != nil {
		t.Fatal(err)
	}
	if strings.ToLower(form.Method) != http.MethodPost || !strings.Contains(form.Action, "/server/auth/login") || !form.CSRF || !form.LoginField {
		t.Fatalf("no-JS login form contract = %+v", form)
	}
}

func TestBrowserAdminLogin(t *testing.T) {
	ctx := newBrowser(t, false)
	saveBrowserArtifacts(t, ctx, "admin-login")
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/server/auth/login"),
		chromedp.WaitVisible(`input[name="identifier"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="identifier"]`, "e2eadmin", chromedp.ByQuery),
		chromedp.SendKeys(`input[name="password"]`, adminPassword, chromedp.ByQuery),
		chromedp.Click(`form[action="/server/auth/login"] button[type="submit"]`, chromedp.ByQuery),
		chromedp.WaitVisible("#admin-nav", chromedp.ByID),
	); err != nil {
		t.Fatalf("admin login flow failed: %v", err)
	}
	var path string
	if err := chromedp.Run(ctx, chromedp.Evaluate("window.location.pathname", &path)); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, "/server/administration") {
		t.Fatalf("admin login landed on %q", path)
	}
	if err := chromedp.Run(ctx,
		chromedp.Click(`form.admin-logout-form button[type="submit"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`input[name="identifier"]`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("admin logout flow failed: %v", err)
	}
}

func TestBrowserUserRegistrationAndLogout(t *testing.T) {
	ctx := newBrowser(t, false)
	saveBrowserArtifacts(t, ctx, "user-registration-logout")
	username := fmt.Sprintf("e2euser%d", time.Now().UnixNano()%1_000_000_000)
	password := "Wthr-E2E-User-Password-93!"
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/server/auth/register"),
		chromedp.WaitVisible(`#registerForm input[name="username"]`, chromedp.ByQuery),
		chromedp.SendKeys(`#registerForm input[name="username"]`, username, chromedp.ByQuery),
		chromedp.SendKeys(`#registerForm input[name="email"]`, username+"@wthr.test", chromedp.ByQuery),
		chromedp.SendKeys(`#registerForm input[name="password"]`, password, chromedp.ByQuery),
		chromedp.SendKeys(`#registerForm input[name="confirm_password"]`, password, chromedp.ByQuery),
		chromedp.Click(`#registerForm button[type="submit"]`, chromedp.ByQuery),
		chromedp.WaitVisible("main#main-content", chromedp.ByQuery),
	); err != nil {
		t.Fatalf("user registration flow failed: %v", err)
	}
	var path string
	if err := chromedp.Run(ctx, chromedp.Evaluate("window.location.pathname", &path)); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, "/users/dashboard") {
		t.Fatalf("user registration landed on %q", path)
	}
	if err := chromedp.Run(ctx,
		chromedp.Click(`form[action="/server/auth/logout"] button[type="submit"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`input[name="identifier"]`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("user logout flow failed: %v", err)
	}
}

func TestBrowserResponsiveThemeAndLanguage(t *testing.T) {
	ctx := newBrowser(t, false)
	saveBrowserArtifacts(t, ctx, "responsive-theme-language")
	if err := chromedp.Run(ctx,
		emulation.SetDeviceMetricsOverride(375, 812, 1, false),
		chromedp.Navigate(baseURL+"/server/about"),
	); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"dark", "light", "auto"} {
		if err := chromedp.Run(ctx,
			chromedp.Evaluate(fmt.Sprintf(`document.cookie = "theme=%s; path=/"`, theme), nil),
			chromedp.Reload(),
			chromedp.WaitReady("body", chromedp.ByQuery),
		); err != nil {
			t.Fatalf("load %s theme: %v", theme, err)
		}
		var actualTheme string
		if err := chromedp.Run(ctx, chromedp.Evaluate("document.documentElement.className", &actualTheme)); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(actualTheme, "theme-"+theme) {
			t.Errorf("theme %q did not render, html class=%q", theme, actualTheme)
		}
	}
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`document.cookie = "lang=ar; path=/"`, nil),
		chromedp.Reload(),
	); err != nil {
		t.Fatal(err)
	}
	var locale struct {
		Lang string `json:"lang"`
		Dir  string `json:"dir"`
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`({lang:document.documentElement.lang, dir:document.documentElement.dir})`, &locale)); err != nil {
		t.Fatal(err)
	}
	if locale.Lang != "ar" || locale.Dir != "rtl" {
		t.Errorf("Arabic document language/direction = %+v", locale)
	}
	var horizontalOverflow bool
	if err := chromedp.Run(ctx, chromedp.Evaluate("document.documentElement.scrollWidth > window.innerWidth", &horizontalOverflow)); err != nil {
		t.Fatal(err)
	}
	if horizontalOverflow {
		t.Error("about page overflows the 375px viewport")
	}
}

func TestBrowserPagesHaveNoJavaScriptExceptions(t *testing.T) {
	ctx := newBrowser(t, false)
	saveBrowserArtifacts(t, ctx, "browser-pages")
	var mu sync.Mutex
	var errors []string
	chromedp.ListenTarget(ctx, func(event any) {
		if exception, ok := event.(*runtime.EventExceptionThrown); ok {
			mu.Lock()
			errors = append(errors, exception.ExceptionDetails.Text)
			mu.Unlock()
		}
	})
	for _, path := range []string{"/server/about", "/server/help", "/server/auth/login"} {
		if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+path), chromedp.WaitReady("body", chromedp.ByQuery)); err != nil {
			t.Fatalf("navigate %s: %v", path, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(errors) != 0 {
		t.Errorf("browser JavaScript exceptions: %s", strings.Join(errors, "; "))
	}
}
