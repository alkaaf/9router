package oauth

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// cursorImportResponse is returned by /api/oauth/cursor/auto-import.
type cursorImportResponse struct {
	Status string `json:"status"`
	JobID  string `json:"jobId"`
}

// inFlightJob tracks in-flight Cursor imports.
type inFlightJob struct {
	mu   sync.Mutex
	jobs map[string]bool
}

var cursorJobTracker = &inFlightJob{jobs: make(map[string]bool)}

func (t *inFlightJob) markInProgress(jobID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.jobs[jobID] = true
}

func (t *inFlightJob) markDone(jobID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.jobs, jobID)
}

// CursorImportRunner runs the import in a goroutine.
type CursorImportRunner func(ctx context.Context, jobID string) error

var cursorImportRunner CursorImportRunner = func(ctx context.Context, jobID string) error {
	_ = ctx
	_ = jobID
	time.Sleep(10 * time.Millisecond)
	return nil
}

// SetCursorImportRunner overrides the background runner.
func SetCursorImportRunner(fn CursorImportRunner) {
	cursorImportRunner = fn
}

// HandleCursorAutoImport implements POST /api/oauth/cursor/auto-import.
func HandleCursorAutoImport(c *Context) (any, error) {
	var body map[string]any
	if len(c.Body) > 0 {
		_ = json.Unmarshal(c.Body, &body)
	}
	_ = body

	jobID := generateID()

	if repo := currentJobRepo(); repo != nil {
		_, _ = repo.Create(c.Ctx, &Job{
			ID:        jobID,
			Status:    "initiated",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		})
	}

	cursorJobTracker.markInProgress(jobID)
	go func() {
		ctx := context.Background()
		defer cursorJobTracker.markDone(jobID)
		_ = cursorImportRunner(ctx, jobID)
	}()

	return cursorImportResponse{
		Status: "initiated",
		JobID:  jobID,
	}, nil
}
