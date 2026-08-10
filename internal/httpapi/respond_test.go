package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestErrorEnvelopeShape(t *testing.T) {
	rec := httptest.NewRecorder()
	emitError(rec, "bad_request", "bad <script>input</script> & more", http.StatusBadRequest)

	body := rec.Body.String()
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content type: got %q", ct)
	}

	var env errorEnvelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("decoding envelope %q: %v", body, err)
	}
	if env.Error.Code != "bad_request" || env.Error.Message != "bad <script>input</script> & more" {
		t.Fatalf("envelope: %+v", env)
	}
	if env.Error.Details == nil || len(env.Error.Details) != 0 {
		t.Fatalf("details: got %v, want an empty object", env.Error.Details)
	}

	// the stable shape serializes details as an empty object, never null, and
	// encoding/json escapes HTML in the message
	if !strings.Contains(body, `"details":{}`) {
		t.Fatalf("details is not an empty object: %s", body)
	}
	if strings.Contains(body, "<script>") {
		t.Fatalf("message is not JSON-escaped: %s", body)
	}
}
