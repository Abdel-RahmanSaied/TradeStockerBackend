package research

import "time"

// RetryBudget limits how many retries start across all jobs: a retry starts
// only while fewer than Capacity retries started within the last Window. A
// retry is any attempt after a job's first, including one recovered after a
// crash or released at shutdown. The count comes from the durable
// ATTEMPT_STARTED events at claim time, so it holds across restarts and
// leadership changes, and retries that all became due during a pause still
// start no faster than the budget. A held retry is never dropped; it waits in
// the queue. First attempts are not limited by the budget, and SEC requests
// stay spaced by the provider's own rate limit either way.
type RetryBudget struct {
	Capacity int
	Window   time.Duration
}

// DefaultRetryBudget allows 6 retry starts in any 10 minutes. The job attempt
// limit (3) and the provider-wide SEC cooldown still apply on top of it.
var DefaultRetryBudget = RetryBudget{Capacity: 6, Window: 10 * time.Minute}

func (s *Service) retryBudget() RetryBudget {
	if s.Retries.Capacity < 1 || s.Retries.Window <= 0 {
		return DefaultRetryBudget
	}
	return s.Retries
}
