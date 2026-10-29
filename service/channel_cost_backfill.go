package service

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type ChannelCostBackfillPayload struct {
	Filter model.LogFilter `json:"filter"`
}

type channelCostBackfillHandler struct{}

func (channelCostBackfillHandler) Type() string { return model.SystemTaskTypeChannelCostBackfill }

func init() { RegisterSystemTaskHandler(channelCostBackfillHandler{}) }

func (channelCostBackfillHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	state := model.ChannelCostBackfillState{Reasons: map[string]int64{}}
	var payload ChannelCostBackfillPayload
	err := task.DecodePayload(&payload)
	if err == nil && (payload.Filter.StartTimestamp <= 0 || payload.Filter.EndTimestamp < payload.Filter.StartTimestamp || payload.Filter.EndTimestamp-payload.Filter.StartTimestamp > 31*24*60*60) {
		err = errors.New("invalid historical cost backfill range")
	}
	var upperID int
	var ratios map[int]*float64
	if err == nil {
		prepareCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		upperID, state.Total, ratios, err = model.PrepareChannelCostBackfill(prepareCtx, payload.Filter)
		cancel()
	}
	if err == nil {
		err = model.UpdateSystemTaskState(task.TaskID, runnerID, state)
	}
	cursor := model.ChannelCostBackfillCursor{}
	lastProgress := time.Now()
	for err == nil {
		batchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		next, delta, batchErr := model.BackfillChannelCostBatch(batchCtx, payload.Filter, upperID, cursor, ratios, task.TaskID)
		cancel()
		if batchErr != nil {
			err = batchErr
			break
		}
		cursor = next
		state.Processed += delta.Processed
		state.Updated += delta.Updated
		state.Skipped += delta.Skipped
		for reason, count := range delta.Reasons {
			state.Reasons[reason] += count
		}
		if state.Total > 0 {
			state.Progress = int(min(int64(99), state.Processed*100/state.Total))
		}
		if delta.Processed < model.ChannelCostBackfillBatchSize {
			state.Progress = 100
			break
		}
		if time.Since(lastProgress) >= 2*time.Second {
			err = model.UpdateSystemTaskState(task.TaskID, runnerID, state)
			lastProgress = time.Now()
		}
		// One worker, at most 100 rows per batch, and no transaction held while
		// yielding. Cancellation (including lost DB lease) interrupts the pause.
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			err = ctx.Err()
		case <-timer.C:
		}
		timer.Stop()
	}
	status, message := model.SystemTaskStatusSucceeded, ""
	if err != nil {
		status = model.SystemTaskStatusFailed
		message = "Historical cost backfill failed. Please retry later."
		common.SysError("historical cost backfill failed: " + err.Error())
	}
	if updateErr := model.UpdateSystemTaskState(task.TaskID, runnerID, state); updateErr != nil {
		status, message = model.SystemTaskStatusFailed, "Historical cost backfill failed. Please retry later."
	}
	if finishErr := model.FinishSystemTask(task.TaskID, runnerID, status, state, message); finishErr != nil {
		common.SysError("historical cost backfill result could not be saved: " + finishErr.Error())
	}
}
