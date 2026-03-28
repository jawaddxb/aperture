// Package auth provides AuthPersistence implementations for Aperture.
// This file implements a Netscape cookie file parser for session import.
package auth

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ParseNetscapeCookies parses a Netscape/Mozilla cookie file from reader.
// It returns the parsed cookies, the unique list of domains encountered, and
// any fatal parse error. Malformed individual lines are skipped gracefully.
//
// Netscape cookie file format (tab-separated columns):
//
//	domain  flag  path  secure  expiry  name  value
//
// Lines starting with '#' or that are blank are skipped.
func ParseNetscapeCookies(reader io.Reader) ([]http.Cookie, []string, error) {
	if reader == nil {
		return nil, nil, fmt.Errorf("cookie_importer: reader must not be nil")
	}

	var cookies []http.Cookie
	domainSet := make(map[string]struct{})

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip blank lines and comment lines.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		c, ok := parseNetscapeLine(line)
		if !ok {
			// Malformed line — skip gracefully.
			continue
		}

		cookies = append(cookies, c)
		domainSet[c.Domain] = struct{}{}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("cookie_importer: scan error: %w", err)
	}

	domains := make([]string, 0, len(domainSet))
	for d := range domainSet {
		domains = append(domains, d)
	}

	return cookies, domains, nil
}

// parseNetscapeLine parses a single Netscape cookie file line.
// Returns the parsed cookie and true on success, or a zero value and false on failure.
func parseNetscapeLine(line string) (http.Cookie, bool) {
	fields := strings.Split(line, "\t")
	if len(fields) < 7 {
		return http.Cookie{}, false
	}

	domain := strings.TrimSpace(fields[0])
	// flag (fields[1]) is the HttpOnly/domain-flag indicator — not used for http.Cookie.
	path := strings.TrimSpace(fields[2])
	secureStr := strings.ToUpper(strings.TrimSpace(fields[3]))
	expiryStr := strings.TrimSpace(fields[4])
	name := strings.TrimSpace(fields[5])
	value := strings.TrimSpace(fields[6])

	if domain == "" || name == "" {
		return http.Cookie{}, false
	}

	secure := secureStr == "TRUE"

	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return http.Cookie{}, false
	}

	c := http.Cookie{
		Name:    name,
		Value:   value,
		Domain:  domain,
		Path:    path,
		Secure:  secure,
		Expires: time.Unix(expiry, 0).UTC(),
	}

	return c, true
}
