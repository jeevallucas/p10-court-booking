package booking

import (
	"fmt"
	"regexp"
	"strings"
)

var mobilePattern = regexp.MustCompile(`^(\+62|62|0)(8[0-9]{8,11})$`)

// NormalizePhone turns the ways Indonesians write a mobile number (0812..., 62812...,
// +62812..., with spaces or dashes) into +62812..., so the owner sees one format.
func NormalizePhone(raw string) (string, error) {
	compact := strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(raw))
	match := mobilePattern.FindStringSubmatch(compact)
	if match == nil {
		return "", fmt.Errorf("%w: %q is not an Indonesian mobile number", ErrInvalidRequest, raw)
	}
	return "+62" + match[2], nil
}
