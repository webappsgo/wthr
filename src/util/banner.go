package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/webappsgo/wthr/src/mode"
)

// bannerInnerWidth is the printable width between the box borders of the
// full (>=80 column) banner, matching the AI.md PART 11 layout.
const bannerInnerWidth = 59

// bannerStartedLayout is the user-facing timestamp format required by AI.md PART 11
const bannerStartedLayout = "January 02, 2006 at 15:04:05 MST"

// getTerminalWidth returns terminal width, defaulting to 80
func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width == 0 {
		return 80
	}
	return width
}

// getBinaryName returns the actual binary name (AI.md PART 8)
func getBinaryName() string {
	return filepath.Base(os.Args[0])
}

// BannerProto resolves the scheme shown on the banner from the TLS state.
// AI.md PART 11: the banner resolves {proto} from TLS, never from request headers.
func BannerProto(useTLS bool) string {
	if useTLS {
		return "https"
	}
	return "http"
}

// BannerListenURL builds the "Listening on" URL for the banner.
// AI.md PART 11/15: {fqdn} is resolved without request headers, :80 and :443
// are stripped, every other port is shown, and bare IPv6 literals are bracketed.
func BannerListenURL(proto, host string, port int) string {
	if host == "" {
		host = "localhost"
	}
	if strings.Count(host, ":") > 1 && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	if (proto == "http" && port == 80) || (proto == "https" && port == 443) {
		return proto + "://" + host
	}
	return fmt.Sprintf("%s://%s:%d", proto, host, port)
}

// bannerStartedAt returns the current time formatted for the banner
func bannerStartedAt() string {
	return time.Now().Format(bannerStartedLayout)
}

// bannerModeIcon returns the mode indicator: locked for production, wrench for development
func bannerModeIcon() string {
	if mode.IsAppModeDev() {
		return GetWrench()
	}
	return GetLock()
}

// bannerBorder renders a full-width box border with the supplied corner runes
func bannerBorder(left, right string) string {
	return left + strings.Repeat("─", bannerInnerWidth) + right
}

// bannerLine renders one padded content row of the full banner
func bannerLine(text string) string {
	return "│  " + padBannerText(text, bannerInnerWidth-2) + "│"
}

// padBannerText pads text to width display columns, accounting for the
// double-width emoji that the console gate may have substituted away
func padBannerText(text string, width int) string {
	visible := bannerDisplayWidth(text)
	if visible >= width {
		return text
	}
	return text + strings.Repeat(" ", width-visible)
}

// bannerDisplayWidth approximates the rendered column count of a banner row.
// Emoji occupy two terminal cells, variation selectors none, and everything
// else (ASCII, box-drawing, punctuation) a single cell.
func bannerDisplayWidth(text string) int {
	width := 0
	for _, r := range text {
		switch {
		case r == 0xFE0F:
			// variation selector renders no cell of its own
		case r >= 0x1F300, r == 0x2705, r == 0x26A0, r == 0x2139:
			width += 2
		default:
			width++
		}
	}
	return width
}

// DisplayFirstRunBanner displays the startup banner with setup information
// AI.md PART 11: Responsive banner adapts to terminal width
// adminBasePath is the resolved "/server/{admin_path}" prefix; the setup wizard lives beneath it at /config/setup
func DisplayFirstRunBanner(port int, useTLS bool, setupToken string, torOnion, i2pAddress, adminBasePath string) {
	proto := BannerProto(useTLS)
	fqdn := GetFQDN()
	listenURL := BannerListenURL(proto, fqdn, port)
	setupURL := listenURL + adminBasePath + "/config/setup"
	binaryName := getBinaryName()

	if !EmojiEnabled() {
		printFirstRunPlain(binaryName, listenURL, setupToken, setupURL)
		return
	}

	// Responsive banner per AI.md PART 11
	switch termWidth := getTerminalWidth(); {
	case termWidth >= 80:
		printFirstRunFull(binaryName, listenURL, torOnion, i2pAddress, setupToken, setupURL)
	case termWidth >= 60:
		printFirstRunCompact(binaryName, listenURL, setupToken, setupURL)
	case termWidth >= 40:
		printFirstRunMinimal(binaryName, listenURL, setupToken)
	default:
		printFirstRunMicro(binaryName, port)
	}
}

func printFirstRunFull(binaryName, listenURL, torOnion, i2pAddress, setupToken, setupURL string) {
	fmt.Println()
	fmt.Println(bannerBorder("╭", "╮"))
	fmt.Println(bannerLine(fmt.Sprintf("%s %s · %s first run", GetRocket(), binaryName, GetPackage())))
	fmt.Println(bannerBorder("├", "┤"))
	fmt.Println(bannerLine(fmt.Sprintf("%s Running in mode: %s", bannerModeIcon(), mode.ModeString())))
	if torOnion != "" {
		fmt.Println(bannerBorder("├", "┤"))
		fmt.Println(bannerLine(fmt.Sprintf("%s Tor:   http://%s", GetOnion(), torOnion)))
	}
	if i2pAddress != "" {
		fmt.Println(bannerBorder("├", "┤"))
		fmt.Println(bannerLine(fmt.Sprintf("%s I2P:   http://%s", GetLink(), i2pAddress)))
	}
	fmt.Println(bannerBorder("├", "┤"))
	fmt.Println(bannerLine(fmt.Sprintf("%s Listening on %s", GetSignal(), listenURL)))
	fmt.Println(bannerLine(fmt.Sprintf("%s Server started on %s", GetOK(), bannerStartedAt())))
	fmt.Println(bannerBorder("╰", "╯"))
	fmt.Println()
	fmt.Println(bannerBorder("┌", "┐"))
	fmt.Println(bannerLine(fmt.Sprintf("%s SETUP REQUIRED", GetLock())))
	fmt.Println(bannerBorder("├", "┤"))
	fmt.Println(bannerLine("Setup Token: " + setupToken))
	fmt.Println(bannerLine("Go to " + setupURL))
	fmt.Println(bannerLine("and enter this token to complete setup."))
	fmt.Println(bannerLine("This token will only be shown ONCE."))
	fmt.Println(bannerBorder("└", "┘"))
	fmt.Println()
}

func printFirstRunCompact(binaryName, listenURL, setupToken, setupURL string) {
	fmt.Printf("%s %s first run\n", GetRocket(), binaryName)
	fmt.Printf("%s Mode: %s\n", bannerModeIcon(), mode.ModeString())
	fmt.Printf("%s Listening: %s\n", GetSignal(), listenURL)
	fmt.Printf("%s Started: %s\n", GetOK(), bannerStartedAt())
	fmt.Println()
	fmt.Printf("%s SETUP REQUIRED\n", GetLock())
	fmt.Printf("Token: %s\n", setupToken)
	fmt.Printf("Go to: %s\n", setupURL)
	fmt.Println("(Token shown ONCE)")
}

func printFirstRunMinimal(binaryName, listenURL, setupToken string) {
	fmt.Printf("%s %s\n", binaryName, mode.ModeString())
	fmt.Println(listenURL)
	fmt.Printf("SETUP: %s\n", setupToken)
}

func printFirstRunMicro(binaryName string, port int) {
	fmt.Printf("%s :%d [SETUP]\n", binaryName, port)
}

func printFirstRunPlain(binaryName, listenURL, setupToken, setupURL string) {
	fmt.Printf("%s first run\n", binaryName)
	fmt.Printf("Mode: %s\n", mode.ModeString())
	fmt.Printf("Listening: %s\n", listenURL)
	fmt.Printf("Started: %s\n", bannerStartedAt())
	fmt.Println()
	fmt.Println("SETUP REQUIRED")
	fmt.Printf("Token: %s\n", setupToken)
	fmt.Printf("Setup URL: %s\n", setupURL)
	fmt.Println("This token will only be shown ONCE.")
}

// DisplayNormalBanner displays the normal startup banner (not first run)
// AI.md PART 8: Must show actual binary name (if renamed)
// AI.md PART 11: Responsive banner adapts to terminal width
func DisplayNormalBanner(version, buildDate string, port int, useTLS bool, torOnion, i2pAddress string) {
	proto := BannerProto(useTLS)
	listenURL := BannerListenURL(proto, GetFQDN(), port)
	binaryName := getBinaryName()

	if !EmojiEnabled() {
		printNormalPlain(binaryName, version, listenURL)
		return
	}

	// Responsive banner per AI.md PART 11
	switch termWidth := getTerminalWidth(); {
	case termWidth >= 80:
		printNormalFull(binaryName, version, buildDate, listenURL, torOnion, i2pAddress)
	case termWidth >= 60:
		printNormalCompact(binaryName, version, listenURL)
	case termWidth >= 40:
		printNormalMinimal(binaryName, version, listenURL)
	default:
		printNormalMicro(binaryName, port)
	}
}

func printNormalFull(binaryName, version, buildDate, listenURL, torOnion, i2pAddress string) {
	fmt.Println()
	fmt.Println(bannerBorder("╭", "╮"))
	fmt.Println(bannerLine(fmt.Sprintf("%s %s · %s %s", GetRocket(), binaryName, GetPackage(), version)))
	fmt.Println(bannerBorder("├", "┤"))
	fmt.Println(bannerLine(fmt.Sprintf("%s Running in mode: %s", bannerModeIcon(), mode.ModeString())))
	if torOnion != "" {
		fmt.Println(bannerBorder("├", "┤"))
		fmt.Println(bannerLine(fmt.Sprintf("%s Tor:   http://%s", GetOnion(), torOnion)))
	}
	if i2pAddress != "" {
		fmt.Println(bannerBorder("├", "┤"))
		fmt.Println(bannerLine(fmt.Sprintf("%s I2P:   http://%s", GetLink(), i2pAddress)))
	}
	fmt.Println(bannerBorder("├", "┤"))
	fmt.Println(bannerLine(fmt.Sprintf("%s Listening on %s", GetSignal(), listenURL)))
	fmt.Println(bannerLine(fmt.Sprintf("%s Server started on %s", GetOK(), bannerStartedAt())))
	fmt.Println(bannerLine("Built: " + buildDate))
	fmt.Println(bannerBorder("╰", "╯"))
	fmt.Println()
}

func printNormalCompact(binaryName, version, listenURL string) {
	fmt.Printf("%s %s v%s\n", GetRocket(), binaryName, version)
	fmt.Printf("%s Mode: %s\n", bannerModeIcon(), mode.ModeString())
	fmt.Printf("%s Listening: %s\n", GetSignal(), listenURL)
	fmt.Printf("%s Started: %s\n", GetOK(), bannerStartedAt())
}

func printNormalMinimal(binaryName, version, listenURL string) {
	fmt.Printf("%s %s\n", binaryName, version)
	fmt.Println(mode.ModeString())
	fmt.Println(listenURL)
}

func printNormalMicro(binaryName string, port int) {
	fmt.Printf("%s :%d\n", binaryName, port)
}

func printNormalPlain(binaryName, version, listenURL string) {
	fmt.Printf("%s v%s\n", binaryName, version)
	fmt.Printf("Mode: %s\n", mode.ModeString())
	fmt.Printf("Listening: %s\n", listenURL)
	fmt.Printf("Started: %s\n", bannerStartedAt())
}

// centerText centers text within a given width
func centerText(text string, width int) string {
	if len(text) >= width {
		return text[:width]
	}
	padding := width - len(text)
	leftPad := padding / 2
	rightPad := padding - leftPad

	result := ""
	for i := 0; i < leftPad; i++ {
		result += " "
	}
	result += text
	for i := 0; i < rightPad; i++ {
		result += " "
	}
	return result
}
