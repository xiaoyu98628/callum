package id

import (
	"strings"
	"testing"
)

func TestValidV7(t *testing.T) {
	valid := "01901234-1234-7234-8234-123456789abc"
	for _, value := range []string{valid, "01901234-1234-7234-b234-123456789abc"} {
		if !ValidV7(value) {
			t.Errorf("拒绝标准 UUIDv7: %q", value)
		}
	}
	for _, value := range []string{"", strings.ToUpper(valid), strings.ReplaceAll(valid, "-", ""), "urn:uuid:" + valid, "{" + valid + "}",
		"01901234-1234-4234-8234-123456789abc", "01901234-1234-7234-0234-123456789abc", "01901234-1234-7234-c234-123456789abc"} {
		if ValidV7(value) {
			t.Errorf("接受非标准 UUIDv7: %q", value)
		}
	}
}
