package mbt

import "testing"

func TestDecode(t *testing.T) {
	tr, err := Load("testdata/orphan.itf.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Steps) < 2 || tr.Steps[0].Action != "init" {
		t.Fatalf("steps = %d, first action %q", len(tr.Steps), tr.Steps[0].Action)
	}
	first := tr.Steps[0].State
	if len(first.Devices) != 2 || len(first.Members) != 2 || len(first.Clusters) != 2 {
		t.Fatalf("init state = %+v", first)
	}
	if d := first.Devices["d1"]; d.Held || d.RV != 0 || d.Token != 0 {
		t.Errorf("d1 at init = %+v", d)
	}
	var sawBind bool
	for _, s := range tr.Steps {
		if s.Action == "landSome" {
			msg, ok := s.Msg()
			if !ok || msg.Dev == "" || msg.Cluster == "" {
				t.Errorf("landSome without a message: %+v", s.Picks)
			}
			sawBind = true
		}
	}
	if !sawBind {
		t.Error("the fixture has no landSome step")
	}
}
