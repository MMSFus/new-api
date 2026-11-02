package service

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/sessionrecorder"
)

// SessionRecorderBackfillState is the progress of a historical index
// backfill, persisted as the system task state and result.
type SessionRecorderBackfillState struct {
	TotalBatches     int    `json:"total_batches"`
	ProcessedBatches int    `json:"processed_batches"`
	IndexedRecords   int64  `json:"indexed_records"`
	SkippedBatches   int    `json:"skipped_batches"`
	FailedBatches    int    `json:"failed_batches"`
	Progress         int    `json:"progress"`
	LastError        string `json:"last_error"`
	Cancelled        bool   `json:"cancelled"`
}

// sessionRecorderBackfillHandler indexes every batch archive in R2 under
// r2_object_prefix, including those archived before the index existed or
// whose local manifest is gone. It runs on the master node through the
// system task runner, apart from the recorder consumer. Finished object keys
// are kept in a checkpoint file under work_dir, so a cancelled run resumes
// where it stopped; failed objects are retried by the next run. Batches the
// live consumer already indexed are skipped without downloading.
type sessionRecorderBackfillHandler struct{}

func (sessionRecorderBackfillHandler) Type() string {
	return model.SystemTaskTypeSessionRecorderBackfill
}

func init() { RegisterSystemTaskHandler(sessionRecorderBackfillHandler{}) }

const sessionRecorderBackfillFailed = "Session recording backfill failed. Please retry later."

func (sessionRecorderBackfillHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	state := SessionRecorderBackfillState{}
	err := runSessionRecorderBackfill(ctx, task, runnerID, &state)
	status, message := model.SystemTaskStatusSucceeded, ""
	switch {
	case state.Cancelled:
		status, message = model.SystemTaskStatusFailed, "Session recording backfill was cancelled"
	case err != nil:
		status, message = model.SystemTaskStatusFailed, sessionRecorderBackfillFailed
		common.SysError("session recorder backfill failed: " + err.Error())
	}
	if updateErr := model.UpdateSystemTaskState(task.TaskID, runnerID, state); updateErr != nil && status == model.SystemTaskStatusSucceeded {
		status, message = model.SystemTaskStatusFailed, sessionRecorderBackfillFailed
	}
	if finishErr := model.FinishSystemTask(task.TaskID, runnerID, status, state, message); finishErr != nil {
		common.SysError("session recorder backfill result could not be saved: " + finishErr.Error())
	}
}

func runSessionRecorderBackfill(ctx context.Context, task *model.SystemTask, runnerID string, state *SessionRecorderBackfillState) error {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap[sessionrecorder.OptionKey]
	common.OptionMapRWMutex.RUnlock()
	settings, err := sessionrecorder.ParseSettings(raw)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(settings.WorkDir) {
		return errors.New("work_dir is not configured")
	}
	workDir := filepath.Clean(settings.WorkDir)
	store, err := sessionrecorder.OpenArchiveStore(settings)
	if err != nil {
		return err
	}
	keys, err := sessionrecorder.ListArchiveObjects(ctx, store, settings.R2ObjectPrefix)
	if err != nil {
		return err
	}
	state.TotalBatches = len(keys)
	if err := model.UpdateSystemTaskState(task.TaskID, runnerID, state); err != nil {
		return err
	}
	checkpoint := sessionrecorder.LoadBackfillCheckpoint(workDir)
	lastSave := time.Now()
	for _, key := range keys {
		if cancelled, err := model.SystemTaskCancelRequested(task.TaskID); err != nil {
			return err
		} else if cancelled {
			state.Cancelled = true
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		state.ProcessedBatches++
		indexed := checkpoint.Done[key]
		if !indexed {
			// The object name carries the batch id; a batch indexed by the
			// live consumer needs no download.
			if indexed, err = model.SessionRecorderBatchIndexed(ctx, sessionrecorder.BatchIDFromObjectKey(key)); err != nil {
				return err
			}
		}
		if indexed {
			state.SkippedBatches++
		} else if count, err := backfillSessionRecorderObject(ctx, store, key); err != nil {
			state.FailedBatches++
			state.LastError = key + ": " + err.Error()
			common.SysError("session recorder backfill: " + state.LastError)
		} else {
			state.IndexedRecords += int64(count)
			checkpoint.Done[key] = true
			if err := sessionrecorder.SaveBackfillCheckpoint(workDir, checkpoint); err != nil {
				return err
			}
		}
		state.Progress = state.ProcessedBatches * 100 / max(1, state.TotalBatches)
		if time.Since(lastSave) >= 2*time.Second {
			if err := model.UpdateSystemTaskState(task.TaskID, runnerID, state); err != nil {
				return err
			}
			lastSave = time.Now()
		}
	}
	state.Progress = 100
	return nil
}

// backfillSessionRecorderObject downloads, parses and indexes one archive.
func backfillSessionRecorderObject(ctx context.Context, store sessionrecorder.ArchiveObjectStore, key string) (int, error) {
	batchCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	_, rows, err := sessionrecorder.IndexArchiveObject(batchCtx, store, key)
	if err != nil {
		return 0, err
	}
	if err := model.ApplySessionRecorderBatch(batchCtx, rows); err != nil {
		return 0, err
	}
	return len(rows), nil
}
