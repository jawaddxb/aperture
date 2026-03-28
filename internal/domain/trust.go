// Package domain defines core interfaces for Aperture.
// This file defines TrustMode for session fingerprint consistency.
package domain

// TrustMode controls how imported sessions handle fingerprint consistency.
type TrustMode string

const (
	// TrustStandard is the normal stealth mode: randomize viewport, canvas noise, etc.
	TrustStandard TrustMode = "standard"

	// TrustPreserve is for imported sessions: no randomisation, consistency is defense.
	TrustPreserve TrustMode = "preserve"
)
