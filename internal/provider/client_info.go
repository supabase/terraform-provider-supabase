package provider

import (
	"runtime"
	"strings"
)

// clientName identifies this provider in the X-Client-Info header.
const clientName = "terraform-provider-supabase"

// userAgentValue returns the User-Agent header value. The leading
// "TFProvider/<version>" token is kept as it has always been so that existing
// log queries that match on it keep working; the Terraform core version is
// appended after it, and omitted when core did not report one.
func userAgentValue(providerVersion, terraformVersion string) string {
	ua := "TFProvider/" + providerVersion
	if terraformVersion != "" {
		ua += " Terraform/" + terraformVersion
	}
	return ua
}

// clientInfoValue returns the X-Client-Info header value in the format shared by
// the Supabase SDKs: "name/version" followed by "; key=value" parameters, in the
// same order supabase-go emits them (runtime, runtime-version, platform,
// platform-architecture).
//
// The runtime is "terraform", not "go": the provider runs as a plugin inside
// Terraform, and the Go toolchain is fixed by the release build, so it can be
// recovered from the provider version. runtime-version is the Terraform core
// version, and is omitted when core did not report one.
//
// "terraform" is not yet listed as a runtime value in the "Standard SDK
// headers" document; it needs a row there before this ships.
func clientInfoValue(providerVersion, terraformVersion string) string {
	var b strings.Builder
	b.WriteString(clientName + "/" + providerVersion)
	b.WriteString("; runtime=terraform")
	if terraformVersion != "" {
		b.WriteString("; runtime-version=" + terraformVersion)
	}
	b.WriteString("; platform=" + platformName(runtime.GOOS))
	b.WriteString("; platform-architecture=" + runtime.GOARCH)
	return b.String()
}

// platformName maps runtime.GOOS onto the platform values the other Supabase
// SDKs emit, using the human-formatted form. It is copied from supabase-go so
// the values line up; values with no mapping are emitted verbatim.
func platformName(goos string) string {
	switch goos {
	case "aix":
		return "AIX"
	case "android":
		return "Android"
	case "darwin":
		return "macOS"
	case "dragonfly":
		return "DragonFly"
	case "freebsd":
		return "FreeBSD"
	case "ios":
		return "iOS"
	case "illumos":
		return "Illumos"
	case "js":
		return "JavaScript"
	case "linux":
		return "Linux"
	case "netbsd":
		return "NetBSD"
	case "openbsd":
		return "OpenBSD"
	case "solaris":
		return "Solaris"
	case "windows":
		return "Windows"
	case "wasip1":
		return "WASIp1"
	default:
		return goos
	}
}
