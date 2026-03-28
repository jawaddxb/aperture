// Package stealth provides uTLS proxy and mode-routing logic for Aperture.
package stealth

import (
	"fmt"

	tls "github.com/refraction-networking/utls"
)

// fingerprintMap maps human-readable fingerprint names to utls ClientHelloIDs.
// "chrome_122" maps to HelloChrome_Auto (the closest available Chrome fingerprint).
// "firefox_121" maps to HelloFirefox_Auto (latest Firefox profile).
var fingerprintMap = map[string]tls.ClientHelloID{
	"chrome_120":  tls.HelloChrome_120,
	"chrome_122":  tls.HelloChrome_Auto, // closest match — Auto tracks latest stable Chrome
	"chrome_auto": tls.HelloChrome_Auto,
	"firefox_121": tls.HelloFirefox_Auto,
	"firefox_auto": tls.HelloFirefox_Auto,
}

// DefaultFingerprint is used when no fingerprint is configured.
const DefaultFingerprint = "chrome_120"

// ResolveFingerprint returns the utls.ClientHelloID for the given name.
// Returns an error if the name is not recognised.
func ResolveFingerprint(name string) (tls.ClientHelloID, error) {
	if name == "" {
		name = DefaultFingerprint
	}
	id, ok := fingerprintMap[name]
	if !ok {
		return tls.ClientHelloID{}, fmt.Errorf("unknown utls fingerprint %q: valid options are chrome_120, chrome_122, firefox_121", name)
	}
	return id, nil
}

// KnownFingerprints returns the list of registered fingerprint names.
func KnownFingerprints() []string {
	names := make([]string, 0, len(fingerprintMap))
	for k := range fingerprintMap {
		names = append(names, k)
	}
	return names
}
