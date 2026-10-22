package guard

import (
	"context"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
)

// The access log is written by JobAccessFlush every 10 seconds (rows still buffered at shutdown are lost:
// at most 10 seconds of log entries) and trimmed by JobAccessPrune once a day.
func init() {
	app.RegisterJob(JobAccessFlush, "@every 10s", FlushAccess)
	app.RegisterJob(JobAccessPrune, "37 4 * * *", func(ctx context.Context, a *app.App) error {
		return PruneAccess(ctx, a, time.Now())
	})
}
