package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// useRequest is one use of the device by a workload.
type useRequest struct {
	// Claim names the claim, as cluster/namespace/name.
	Claim string `json:"claim"`
	// Token is the fencing token of the claim.
	Token int64 `json:"token"`
}

// useRecord is one use as the device saw it.
type useRecord struct {
	Time     time.Time `json:"time"`
	Claim    string    `json:"claim"`
	Token    int64     `json:"token"`
	Accepted bool      `json:"accepted"`
	Highest  int64     `json:"highest"`
}

// device is a gatekeeper that checks fencing tokens, spec 6.6.
type device struct {
	mu      sync.Mutex
	highest int64
	log     []useRecord
	now     func() time.Time
}

//= spec/solas.md#6-6-device-use
//# A device that cannot check tokens MUST sit behind a gatekeeper that
//# checks them.

// use applies the token rules and records the use.
func (d *device) use(req useRequest) useRecord {
	d.mu.Lock()
	defer d.mu.Unlock()
	//= spec/solas.md#6-6-device-use
	//# The device MUST reject a use with a token lower than the highest token
	//# that it has accepted.

	//= spec/solas.md#6-6-device-use
	//# The device MUST accept a use with a token equal to or higher than the
	//# highest token that it has accepted, and record that token as the highest.

	//= spec/solas.md#6-6-device-use
	//# The device MUST remember the highest token that it has accepted.
	accepted := req.Token >= d.highest
	if accepted {
		d.highest = req.Token
	}
	rec := useRecord{Time: d.now(), Claim: req.Claim, Token: req.Token, Accepted: accepted, Highest: d.highest}
	d.log = append(d.log, rec)
	return rec
}

func (d *device) records() []useRecord {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]useRecord(nil), d.log...)
}

// handler serves POST /use and GET /log.
func (d *device) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /use", func(w http.ResponseWriter, r *http.Request) {
		var req useRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Claim == "" {
			http.Error(w, "want JSON {claim, token}", http.StatusBadRequest)
			return
		}
		rec := d.use(req)
		w.Header().Set("Content-Type", "application/json")
		if !rec.Accepted {
			w.WriteHeader(http.StatusConflict)
		}
		_ = json.NewEncoder(w).Encode(rec)
	})
	mux.HandleFunc("GET /log", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(d.records())
	})
	return mux
}
