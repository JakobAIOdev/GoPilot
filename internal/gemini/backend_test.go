package gemini

import (
	"strings"
	"testing"
)

func TestDecodeAPIErrorJSONPayload(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"error":{"code":429,"message":"Resource exhausted","status":"RESOURCE_EXHAUSTED"}}`)
	err := decodeAPIError("429 Too Many Requests", raw)
	if err == nil {
		t.Fatal("expected error")
	}
	text := err.Error()
	if !strings.Contains(text, "Resource exhausted") || !strings.Contains(text, "RESOURCE_EXHAUSTED") {
		t.Fatalf("unexpected error text: %q", text)
	}
}

func TestDecodeAPIErrorNonJSONPayload(t *testing.T) {
	t.Parallel()

	err := decodeAPIError("502 Bad Gateway", []byte("<html>bad gateway</html>"))
	if err == nil {
		t.Fatal("expected error")
	}
	text := err.Error()
	if !strings.Contains(text, "502 Bad Gateway") || !strings.Contains(text, "bad gateway") {
		t.Fatalf("unexpected error text: %q", text)
	}
}
