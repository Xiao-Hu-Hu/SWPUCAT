package business

import (
	"regexp"
	"strings"
)

var untrustedIdentifierPattern = regexp.MustCompile(`\b[A-Za-z0-9]+(?:_[A-Za-z0-9]+){2,}\b`)

var sensitiveIdentifierMarkers = []string{
	"_CANARY_",
	"_SECRET_",
	"_TOKEN_",
	"_PASSWORD_",
	"_CREDENTIAL_",
}

// RedactUntrustedSecrets removes secret-shaped identifiers from user,
// conversation, knowledge and tool text before it is sent to the model. The
// heuristic deliberately requires a multi-part identifier and a sensitive
// marker, so ordinary prose and documented configuration keys remain useful.
func RedactUntrustedSecrets(value string) string {
	return untrustedIdentifierPattern.ReplaceAllStringFunc(value, func(candidate string) string {
		upper := strings.ToUpper(candidate)
		for _, marker := range sensitiveIdentifierMarkers {
			if strings.Contains(upper, marker) {
				return "[敏感标识已隐藏]"
			}
		}
		return candidate
	})
}
