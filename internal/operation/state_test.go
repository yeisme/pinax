package operation

import "testing"

func TestOperationStateTransitionMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		from       Status
		to         Status
		retryable  bool
		replaySafe bool
		want       bool
	}{
		{name: "accepted to applying", from: StatusAccepted, to: StatusApplying, want: true},
		{name: "accepted cannot skip apply", from: StatusAccepted, to: StatusSucceeded},
		{name: "applying succeeded", from: StatusApplying, to: StatusSucceeded, want: true},
		{name: "applying failed", from: StatusApplying, to: StatusFailed, want: true},
		{name: "applying ambiguous", from: StatusApplying, to: StatusReconcileRequired, want: true},
		{name: "reconcile stays fail safe", from: StatusReconcileRequired, to: StatusReconcileRequired, want: true},
		{name: "reconcile proves success", from: StatusReconcileRequired, to: StatusSucceeded, want: true},
		{name: "failed cannot retry by default", from: StatusFailed, to: StatusApplying},
		{name: "failed retry needs both facts", from: StatusFailed, to: StatusApplying, retryable: true},
		{name: "failed retry safe", from: StatusFailed, to: StatusApplying, retryable: true, replaySafe: true, want: true},
		{name: "succeeded terminal", from: StatusSucceeded, to: StatusApplying},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := CanTransition(test.from, test.to, test.retryable, test.replaySafe); got != test.want {
				t.Fatalf("CanTransition(%q, %q) = %v, want %v", test.from, test.to, got, test.want)
			}
		})
	}
}

func TestOperationStatusClassification(t *testing.T) {
	t.Parallel()

	for _, status := range []Status{StatusAccepted, StatusApplying, StatusSucceeded, StatusFailed, StatusReconcileRequired} {
		if !StatusKnown(status) {
			t.Fatalf("stable status %q is unknown", status)
		}
	}
	if StatusKnown("future_status") || StatusTerminal(StatusReconcileRequired) || !StatusTerminal(StatusSucceeded) || !StatusTerminal(StatusFailed) {
		t.Fatal("status classification is not fail-safe")
	}
}
