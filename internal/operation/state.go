package operation

func CanTransition(from, to Status, retryable, replaySafe bool) bool {
	switch from {
	case StatusAccepted:
		return to == StatusApplying
	case StatusApplying:
		return to == StatusSucceeded || to == StatusFailed || to == StatusReconcileRequired
	case StatusReconcileRequired:
		return to == StatusSucceeded || to == StatusFailed || to == StatusReconcileRequired
	case StatusFailed:
		return to == StatusApplying && retryable && replaySafe
	default:
		return false
	}
}

func StatusKnown(status Status) bool {
	switch status {
	case StatusAccepted, StatusApplying, StatusSucceeded, StatusFailed, StatusReconcileRequired:
		return true
	default:
		return false
	}
}

func StatusTerminal(status Status) bool {
	return status == StatusSucceeded || status == StatusFailed
}
