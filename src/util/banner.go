package utils

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/term"
)

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

// DisplayFirstRunBanner displays the startup banner with setup information
// AI.md PART 11: Responsive banner adapts to terminal width
func DisplayFirstRunBanner(port int, setupToken string, isDockerized bool, torOnion string) {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}

	localURL := fmt.Sprintf("http://%s:%d", hostname, port)
	dockerURL := ""
	if isDockerized {
		if gwIP := GetDockerGatewayIP(); gwIP != "" {
			dockerURL = fmt.Sprintf("http://%s:%d", gwIP, port)
		}
	}

	binaryName := getBinaryName()
	termWidth := getTerminalWidth()

	// Responsive banner per AI.md PART 11
	switch {
	case termWidth >= 80:
		printFirstRunFull(binaryName, localURL, dockerURL, torOnion, setupToken, isDockerized)
	case termWidth >= 60:
		printFirstRunCompact(binaryName, localURL, setupToken)
	case termWidth >= 40:
		printFirstRunMinimal(binaryName, port, setupToken)
	default:
		printFirstRunMicro(binaryName, port)
	}
}

func printFirstRunFull(binaryName, localURL, dockerURL, torOnion, setupToken string, isDockerized bool) {
	// AI.md PART 8: Use emoji fallbacks when NO_COLOR set or TERM=dumb
	rocket := GetRocket()
	globe := GetGlobe()
	docker := GetDocker()
	onion := GetOnion()
	ok := GetOK()
	lock := GetLock()

	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Printf("║%s║\n", centerText(fmt.Sprintf("%s %s - First Run Setup", rocket, binaryName), 65))
	fmt.Println("║                                                               ║")
	fmt.Printf("║  %s Local:  %-48s ║\n", globe, localURL)

	if isDockerized && dockerURL != "" {
		fmt.Printf("║  %s Docker: %-48s ║\n", docker, dockerURL)
	}

	if torOnion != "" {
		fmt.Printf("║  %s Tor:    %-48s ║\n", onion, torOnion)
	}

	fmt.Println("║                                                               ║")
	fmt.Printf("║  %s Server started (before setup)                            ║\n", ok)
	fmt.Println("║                                                               ║")
	fmt.Printf("║  %s Setup Token: %-43s ║\n", lock, setupToken)
	fmt.Println("║     Use at /admin/server/setup (ONE TIME)                    ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

func printFirstRunCompact(binaryName, localURL, setupToken string) {
	fmt.Printf("%s %s - First Run\n", GetRocket(), binaryName)
	fmt.Printf("%s %s\n", GetGlobe(), localURL)
	fmt.Printf("%s Token: %s\n", GetLock(), setupToken)
}

func printFirstRunMinimal(binaryName string, port int, setupToken string) {
	fmt.Printf("%s :%d (first run)\n", binaryName, port)
	fmt.Printf("Token: %s\n", setupToken)
}

func printFirstRunMicro(binaryName string, port int) {
	fmt.Printf("%s :%d\n", binaryName, port)
}

// DisplayNormalBanner displays the normal startup banner (not first run)
// AI.md PART 8: Must show actual binary name (if renamed)
// AI.md PART 11: Responsive banner adapts to terminal width
func DisplayNormalBanner(version, buildDate string, port int, isDockerized bool, torOnion string) {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}

	localURL := fmt.Sprintf("http://%s:%d", hostname, port)
	dockerURL := ""
	if isDockerized {
		// AI.md: Detect Docker gateway at runtime, not hardcoded
		if gwIP := GetDockerGatewayIP(); gwIP != "" {
			dockerURL = fmt.Sprintf("http://%s:%d", gwIP, port)
		}
	}

	binaryName := getBinaryName()
	termWidth := getTerminalWidth()

	// Responsive banner per AI.md PART 11
	switch {
	case termWidth >= 80:
		printNormalFull(binaryName, version, buildDate, localURL, dockerURL, torOnion, isDockerized)
	case termWidth >= 60:
		printNormalCompact(binaryName, version, localURL)
	case termWidth >= 40:
		printNormalMinimal(binaryName, version, port)
	default:
		printNormalMicro(binaryName, port)
	}
}

func printNormalFull(binaryName, version, buildDate, localURL, dockerURL, torOnion string, isDockerized bool) {
	// AI.md PART 8: Use emoji fallbacks when NO_COLOR set or TERM=dumb
	sun := GetSun()
	globe := GetGlobe()
	docker := GetDocker()
	onion := GetOnion()
	ok := GetOK()
	width := 65

	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Printf("║%s║\n", centerText(fmt.Sprintf("%s  %s v%s", sun, binaryName, version), width))
	fmt.Println("║                                                               ║")
	fmt.Printf("║  %s Local:  %-48s ║\n", globe, localURL)

	if isDockerized && dockerURL != "" {
		fmt.Printf("║  %s Docker: %-48s ║\n", docker, dockerURL)
	}

	if torOnion != "" {
		fmt.Printf("║  %s Tor:    %-48s ║\n", onion, torOnion)
	}

	fmt.Println("║                                                               ║")
	fmt.Printf("║  Built: %-54s ║\n", buildDate)
	fmt.Println("║                                                               ║")
	fmt.Printf("║  %s Server ready                                             ║\n", ok)
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

func printNormalCompact(binaryName, version, localURL string) {
	fmt.Printf("%s %s v%s\n", GetSun(), binaryName, version)
	fmt.Printf("%s %s\n", GetGlobe(), localURL)
	fmt.Printf("%s Server ready\n", GetOK())
}

func printNormalMinimal(binaryName, version string, port int) {
	fmt.Printf("%s v%s :%d\n", binaryName, version, port)
	fmt.Println("Ready")
}

func printNormalMicro(binaryName string, port int) {
	fmt.Printf("%s :%d\n", binaryName, port)
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
