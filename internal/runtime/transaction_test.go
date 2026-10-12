package runtime

import (
	"errors"
	"testing"
)

func TestEveryPolicyFailureRollsBackOnlyCompletedSteps(t *testing.T) {
	plan := ProxyPlan(proxyFixture(), "", nil)
	for failure := 0; failure <= len(plan); failure++ {
		calls, saved := 0, 0
		active := map[int]bool{}
		done, failed, e := ApplyPolicy(plan, func(next []Operation) error { saved = len(next); return nil }, func(op Operation) error {
			if saved != calls+1 {
				t.Fatal("mutation before checkpoint")
			}
			if calls == failure {
				return errors.New("injected failure")
			}
			active[calls] = true
			calls++
			return nil
		})
		if failure < len(plan) && (e == nil || failed == nil) {
			t.Fatal("lost error")
		}
		if len(done) != failure {
			t.Fatal("incorrect completed set", failure, len(done))
		}
		for i := len(done) - 1; i >= 0; i-- {
			if len(done[i].Undo) == 0 {
				t.Fatal("missing inverse")
			}
			delete(active, i)
		}
		if len(active) != 0 {
			t.Fatal("rollback left completed operation")
		}
	}
	mutated := false
	_, _, e := ApplyPolicy(plan, func([]Operation) error { return errors.New("disk full") }, func(Operation) error { mutated = true; return nil })
	if e == nil || mutated {
		t.Fatal("checkpoint failure mutated network")
	}
}
