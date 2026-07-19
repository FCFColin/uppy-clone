// Package testutil provides shared test helpers for backend tests.
package testutil

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

// DecodeJSONBody decodes the JSON body of an httptest.ResponseRecorder into v.
// On decode error it fails the test with a descriptive message including the
// raw body — replacing the verbose `json.NewDecoder(w.Body).Decode` +
// `t.Fatalf` pattern repeated across handler tests.
func DecodeJSONBody(t *testing.T, w *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(w.Body).Decode(v); err != nil {
		t.Fatalf("decode response body: %v (body=%q)", err, w.Body.String())
	}
}
