package fingerprint

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var browserVersionPattern = regexp.MustCompile(`(?i)(?:Chrome|Chromium|HeadlessChrome|Edg|Brave)/([0-9]+(?:\.[0-9]+){0,3})`)

func BuildIdentity(profile Profile, product, nativeUserAgent string) (Identity, error) {
	if err := profile.Validate(); err != nil {
		return Identity{}, err
	}
	version := extractBrowserVersion(product + " " + nativeUserAgent)
	if version == "" {
		return Identity{}, errors.New("Chromium did not report a recognizable browser version")
	}
	major := strings.Split(version, ".")[0]
	brand := "Google Chrome"
	productLower := strings.ToLower(product)
	uaLower := strings.ToLower(nativeUserAgent)
	switch {
	case strings.Contains(productLower, "edge") || strings.Contains(uaLower, "edg/"):
		brand = "Microsoft Edge"
	case strings.Contains(productLower, "brave"):
		brand = "Brave"
	case strings.Contains(productLower, "chromium"):
		brand = "Chromium"
	}

	return Identity{
		Profile: profile, UserAgent: userAgentFor(profile, version),
		BrowserVersion: version, BrowserMajor: major,
		PrimaryBrand: brand, PrimaryBrandVersion: version,
	}, nil
}

func extractBrowserVersion(value string) string {
	match := browserVersionPattern.FindStringSubmatch(value)
	if len(match) != 2 {
		return ""
	}
	parts := strings.Split(match[1], ".")
	for len(parts) < 4 {
		parts = append(parts, "0")
	}
	return strings.Join(parts[:4], ".")
}

func userAgentFor(profile Profile, version string) string {
	var platformToken string
	switch profile.Platform {
	case "Windows":
		platformToken = "Windows NT 10.0; Win64; x64"
	case "macOS":
		platformToken = "Macintosh; Intel Mac OS X 10_15_7"
	case "Linux":
		platformToken = "X11; Linux x86_64"
	default:
		platformToken = fmt.Sprintf("%s; %s", profile.Platform, profile.Architecture)
	}
	return "Mozilla/5.0 (" + platformToken + ") AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + version + " Safari/537.36"
}
