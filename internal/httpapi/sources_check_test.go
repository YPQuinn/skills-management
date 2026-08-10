package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestSourcesCheckStrictBody(t *testing.T) {
	ts := readyServer(t, nil)
	root := t.TempDir()
	writeAPISkill(t, root, "alpha")
	body := fmt.Sprintf(`{"kind": "local", "location": %q}`, root)
	resp := ts.do(t, "POST", "/api/v1/sources", body, "application/json", "", "")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var created sourceJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &created); err != nil {
		t.Fatal(err)
	}

	show := func() sourceJSON {
		t.Helper()
		resp := ts.do(t, "GET", fmt.Sprintf("/api/v1/sources/%d", created.ID), "", "", "", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("show: got %d, body %s", resp.StatusCode, readBody(t, resp))
		}
		var s sourceJSON
		if err := json.Unmarshal([]byte(readBody(t, resp)), &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	before := show()

	// malformed, trailing, unknown, non-object, and empty bodies are bad
	// requests and must not advance check state
	for _, bad := range []string{`{`, `{} {}`, `{"bogus": 1}`, `null`, `[]`, `"string"`, ``} {
		resp := ts.do(t, "POST", fmt.Sprintf("/api/v1/sources/%d/check", created.ID), bad, "application/json", "", "")
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("check body %q: got %d, want 400 (body %s)", bad, resp.StatusCode, readBody(t, resp))
			continue
		}
		if env := decodeError(t, readBody(t, resp)); env.Error.Code != "bad_request" {
			t.Errorf("check body %q: got code %q, want bad_request", bad, env.Error.Code)
		}
	}
	after := show()
	if after.LastCheckedAt != before.LastCheckedAt || after.LastCheckStartedAt != before.LastCheckStartedAt {
		t.Fatalf("rejected check bodies must not advance check times: before %q/%q, after %q/%q",
			before.LastCheckedAt, before.LastCheckStartedAt, after.LastCheckedAt, after.LastCheckStartedAt)
	}
	if after.UpdatedAt != before.UpdatedAt {
		t.Fatalf("rejected check bodies must not advance updated_at: before %q, after %q", before.UpdatedAt, after.UpdatedAt)
	}
	if after.LastCheckResult != before.LastCheckResult || len(after.Inventory) != len(before.Inventory) {
		t.Fatalf("rejected check bodies must not change check state: before %+v, after %+v", before, after)
	}

	// a valid object body still checks and replaces the Inventory
	writeAPISkill(t, root, "beta")
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/sources/%d/check", created.ID), `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid check: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var checked sourceJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &checked); err != nil {
		t.Fatal(err)
	}
	if checked.EntryCount != 2 || len(checked.Inventory) != 2 {
		t.Fatalf("a valid check must replace the Inventory: %+v", checked)
	}
	if checked.LastCheckResult != "ok" {
		t.Fatalf("valid check result: got %q", checked.LastCheckResult)
	}
}
