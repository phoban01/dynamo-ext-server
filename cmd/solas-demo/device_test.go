package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func post(t *testing.T, h http.Handler, claim string, token int64) (int, useRecord) {
	t.Helper()
	body, _ := json.Marshal(useRequest{Claim: claim, Token: token})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/use", bytes.NewReader(body)))
	var rec useRecord
	_ = json.Unmarshal(rr.Body.Bytes(), &rec)
	return rr.Code, rec
}

//= spec/solas.md#6-6-device-use
//= type=test
//# The device MUST reject a use with a token lower than the highest token
//# that it has accepted.

//= spec/solas.md#6-6-device-use
//= type=test
//# The device MUST accept a use with a token equal to or higher than the
//# highest token that it has accepted, and record that token as the highest.

func TestDeviceRejectsAStaleToken(t *testing.T) {
	h := (&device{now: time.Now}).handler()
	steps := []struct {
		claim   string
		token   int64
		want    int
		highest int64
	}{
		{"b/ns/job", 1, http.StatusOK, 1},       // the first holder
		{"b/ns/job", 1, http.StatusOK, 1},       // the same holder again
		{"c/ns/job", 2, http.StatusOK, 2},       // the new holder after a reclaim
		{"b/ns/job", 1, http.StatusConflict, 2}, // the old holder wakes up
		{"c/ns/job", 2, http.StatusOK, 2},       // the new holder goes on
	}
	for i, s := range steps {
		code, rec := post(t, h, s.claim, s.token)
		if code != s.want {
			t.Errorf("step %d (%s, token %d): code %d, want %d", i, s.claim, s.token, code, s.want)
		}
		if rec.Highest != s.highest {
			t.Errorf("step %d: highest %d, want %d", i, rec.Highest, s.highest)
		}
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/log", nil))
	var log []useRecord
	if err := json.Unmarshal(rr.Body.Bytes(), &log); err != nil || len(log) != len(steps) {
		t.Fatalf("log: %v, %d records", err, len(log))
	}
	if log[3].Accepted || log[3].Claim != "b/ns/job" {
		t.Errorf("record 3 = %+v, want a rejected use by b", log[3])
	}
}

func TestDeviceRejectsABadRequest(t *testing.T) {
	h := (&device{now: time.Now}).handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/use", bytes.NewReader([]byte("{}"))))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("code %d, want 400", rr.Code)
	}
}
