package migrate

import (
	"context"
	"testing"
	"time"

	clocktesting "k8s.io/utils/clock/testing"
)

//= spec/solas.md#12-3-a-store-that-cannot-be-sealed
//= type=test
//# The tool MUST NOT copy when a `Member` renewed in that time, or when a
//# `Device` or a `Member` changed.

func TestQuiet(t *testing.T) {
	const d = 30 * time.Second
	cases := map[string]struct {
		before, after map[string]string
		wantErr       bool
	}{
		"no change":     {map[string]string{"members/a": "5", "devices/d1": "7"}, map[string]string{"members/a": "5", "devices/d1": "7"}, false},
		"a renew":       {map[string]string{"members/a": "5"}, map[string]string{"members/a": "9"}, true},
		"a new member":  {map[string]string{}, map[string]string{"members/b": "3"}, true},
		"a deleted one": {map[string]string{"devices/d1": "7"}, map[string]string{}, true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			clk := clocktesting.NewFakeClock(time.Unix(0, 0))
			calls := 0
			versions := func(context.Context) (map[string]string, error) {
				calls++
				if calls == 1 {
					return c.before, nil
				}
				return c.after, nil
			}
			done := make(chan error, 1)
			go func() { done <- Quiet(context.Background(), versions, d, clk) }()
			// The second list must wait for d on the tool's clock.
			for !clk.HasWaiters() {
				time.Sleep(time.Millisecond)
			}
			clk.Step(d - time.Second)
			select {
			case err := <-done:
				t.Fatalf("Quiet returned %v before d passed", err)
			case <-time.After(20 * time.Millisecond):
			}
			clk.Step(time.Second)
			if err := <-done; (err != nil) != c.wantErr {
				t.Errorf("Quiet = %v, want error %v", err, c.wantErr)
			}
		})
	}
}
