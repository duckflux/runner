package engine

import (
	"context"
	"math"
	"time"

	"github.com/duckflux/runner/internal/model"
)

// defaultBackoff is the initial retry sleep duration when RetryConfig.Backoff is zero.
const defaultBackoff = time.Second

// defaultFactor is the exponential multiplier when RetryConfig.Factor is zero or unset.
const defaultFactor = 2.0

// executeWithRetry calls execute once; on failure it retries up to retry.Max times
// using exponential backoff. The backoff delay before attempt i (0-indexed) is:
//
//	delay = backoff * factor^i
//
// where backoff defaults to 1s and factor defaults to 2.0 when not configured.
//
// Each sleep is interrupted early if ctx is cancelled, returning ctx.Err() immediately.
//
// Returns (output, retriesPerformed, error):
//   - retriesPerformed is 0 when the first call succeeds.
//   - retriesPerformed equals the number of retry attempts made after the first call.
//   - error is nil on success, non-nil when all attempts fail.
func executeWithRetry(ctx context.Context, execute func() (any, error), retry *model.RetryConfig) (any, int, error) {
	out, err := execute()
	if err == nil {
		return out, 0, nil
	}

	// No retry config or max <= 0 means no retries — return the first failure.
	if retry == nil || retry.Max <= 0 {
		return nil, 0, err
	}

	backoff := defaultBackoff
	if retry.Backoff.Duration > 0 {
		backoff = retry.Backoff.Duration
	}
	factor := defaultFactor
	if retry.Factor > 0 {
		factor = retry.Factor
	}

	for attempt := 0; attempt < retry.Max; attempt++ {
		delay := time.Duration(float64(backoff) * math.Pow(factor, float64(attempt)))
		select {
		case <-ctx.Done():
			return nil, attempt, ctx.Err()
		case <-time.After(delay):
		}

		out, err = execute()
		if err == nil {
			return out, attempt + 1, nil
		}
	}

	return nil, retry.Max, err
}
