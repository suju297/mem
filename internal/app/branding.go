package app

import (
	"io"
	"strings"
)

// Terminal logo: simple MEM wordmark for terminal entry surfaces.
var memLogoLines = []string{
	" __  __ _____ __  __",
	"|  \\/  | ____|  \\/  |",
	"| |\\/| |  _| | |\\/| |",
	"| |  | | |___| |  | |",
	"|_|  |_|_____|_|  |_|",
}

func writeMemLogo(w io.Writer) {
	io.WriteString(w, renderMemLogo(shouldColorize(w)))
}

func renderMemLogo(useColor bool) string {
	logo := strings.Join(memLogoLines, "\n")
	if !useColor {
		return logo
	}
	return colorizeLogo(logo)
}

func colorizeLogo(text string) string {
	const magenta = "\x1b[35m"
	const bold = "\x1b[1m"
	const reset = "\x1b[0m"
	return bold + magenta + text + reset
}
