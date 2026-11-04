package controller

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var errSub2APIRatioUnsupported = errors.New("Unsupported Sub2API billing response")

type sub2APIRatioProbe struct {
	GroupRateMultiplier    *float64 `json:"group_rate_multiplier"`
	UserRateMultiplier     *float64 `json:"user_rate_multiplier"`
	ResolvedRateMultiplier *float64 `json:"resolved_rate_multiplier"`
	PeakRateEnabled        bool     `json:"peak_rate_enabled"`
	ObservedAt             string   `json:"observed_at"`
	InRange                bool     `json:"in_range"`
}

func upstreamRatioEndpointURL(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(path, "/v1") {
		parsed.Path = path + "/sub2api/billing"
	} else {
		parsed.Path = path + "/v1/sub2api/billing"
	}
	parsed.RawPath = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func probeSub2APIRatio(channel *model.Channel) (*sub2APIRatioProbe, error) {
	return probeSub2APIRatioContext(context.Background(), channel)
}

func probeSub2APIRatioContext(ctx context.Context, channel *model.Channel) (*sub2APIRatioProbe, error) {
	if eligible, reason := model.UpstreamRatioSyncEligibility(channel); !eligible {
		return nil, common.NewMessage(reason)
	}
	endpoint, err := upstreamRatioEndpointURL(channel.GetBaseURL())
	if err != nil {
		return nil, err
	}
	key := strings.TrimSpace(channel.Key)
	if channel.ChannelInfo.IsMultiKey {
		keys := channel.GetKeys()
		if strings.HasPrefix(key, "[") {
			if err := common.UnmarshalJsonStr(key, &keys); err != nil {
				return nil, common.NewMessage("Invalid Sub2API channel keys")
			}
		}
		if len(keys) == 0 {
			return nil, common.NewMessage("Sub2API channel is missing key")
		}
		key = strings.TrimSpace(keys[0])
	}
	if key == "" {
		return nil, common.NewMessage("Sub2API channel is missing key")
	}
	body, err := GetResponseBody(http.MethodGet, endpoint, channel, GetAuthHeader(key), responseBodyLimits{Timeout: 10 * time.Second, MaxBytes: 64 << 10, Context: ctx})
	if err != nil {
		if strings.Contains(err.Error(), "status code: 404") {
			return nil, errSub2APIRatioUnsupported
		}
		return nil, sanitizeAdvancedCustomRequestError(err, key, endpoint)
	}
	var validated json.RawMessage
	if err := common.Unmarshal(body, &validated); err != nil {
		return nil, common.NewMessage("Invalid Sub2API billing response")
	}
	if common.GetJsonType(validated) != "object" {
		return nil, errSub2APIRatioUnsupported
	}
	var envelope map[string]json.RawMessage
	if err := common.Unmarshal(body, &envelope); err != nil {
		return nil, common.NewMessage("Invalid Sub2API billing response")
	}
	var object, scope string
	var version int
	if common.Unmarshal(envelope["object"], &object) != nil || common.Unmarshal(envelope["schema_version"], &version) != nil || common.Unmarshal(envelope["billing_scope"], &scope) != nil || object != "sub2api.key_billing" || version != 1 || scope != "token" {
		return nil, errSub2APIRatioUnsupported
	}
	var response sub2APIRatioProbe
	if err := common.Unmarshal(body, &response); err != nil {
		return nil, common.NewMessage("Invalid Sub2API billing response")
	}
	if response.ResolvedRateMultiplier == nil || !model.UpstreamCostRatioInRange(dto.ChannelSettings{}, *response.ResolvedRateMultiplier) {
		return nil, common.NewMessage("Invalid Sub2API resolved rate multiplier")
	}
	response.InRange = model.UpstreamCostRatioInRange(channel.GetSetting(), *response.ResolvedRateMultiplier)
	return &response, nil
}

func syncSub2APIRatio(channel *model.Channel, automatic bool) (model.ChannelCostRatioSyncResult, error) {
	return syncSub2APIRatioContext(context.Background(), channel, automatic)
}

func syncSub2APIRatioContext(ctx context.Context, channel *model.Channel, automatic bool) (model.ChannelCostRatioSyncResult, error) {
	if eligible, reason := model.UpstreamRatioSyncEligibility(channel); !eligible {
		return model.ChannelCostRatioSyncResult{}, common.NewMessage(reason)
	}
	probedChannel := *channel
	baseURL := channel.GetBaseURL()
	probedChannel.BaseURL = &baseURL
	probe, err := probeSub2APIRatioContext(ctx, channel)
	status, message := "success", ""
	var value *float64
	var info string
	if err != nil {
		status, message = "failed", err.Error()
		if errors.Is(err, errSub2APIRatioUnsupported) {
			status = "unsupported"
		}
	} else {
		value = probe.ResolvedRateMultiplier
		encoded, err := common.Marshal(map[string]any{
			"group_rate_multiplier":    probe.GroupRateMultiplier,
			"user_rate_multiplier":     probe.UserRateMultiplier,
			"resolved_rate_multiplier": probe.ResolvedRateMultiplier,
			"observed_at":              probe.ObservedAt,
		})
		if err != nil {
			return model.ChannelCostRatioSyncResult{}, err
		}
		info = string(encoded)
	}
	return model.ApplyChannelCostRatioSync(&probedChannel, value, status, message, info, automatic)
}

func channelForCostRatio(c *gin.Context) (*model.Channel, error) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return nil, err
	}
	return model.GetChannelById(id, true)
}

func ProbeChannelUpstreamCostRatio(c *gin.Context) {
	channel, err := channelForCostRatio(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	probe, err := probeSub2APIRatio(channel)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": probe})
}

func SyncChannelUpstreamCostRatio(c *gin.Context) {
	channel, err := channelForCostRatio(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := syncSub2APIRatio(channel, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	success := result.Status != "failed" && result.Status != "unsupported" && result.Status != "stale"
	c.JSON(http.StatusOK, gin.H{"success": success, "message": result.Error, "data": result})
}

func GetChannelUpstreamCostRatioHistory(c *gin.Context) {
	channel, err := channelForCostRatio(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = max(1, page), max(1, min(100, pageSize))
	items, total, status, err := model.GetChannelCostRatioHistory(channel.Id, page, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{
		"items": items, "total": total, "page": page, "page_size": pageSize, "sync_status": status,
	}})
}

func AutomaticallySyncChannelCostRatios() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if !common.IsMasterNode {
			continue
		}
		channels, err := model.GetAllChannels(0, 0, true, false)
		if err != nil {
			continue
		}
		for _, channel := range channels {
			if eligible, _ := model.UpstreamRatioSyncEligibility(channel); !eligible || !channel.GetSetting().UpstreamCostRatioSyncEnabled {
				continue
			}
			var state model.ChannelCostRatioSyncStatus
			if err := model.DB.Where("channel_id = ?", channel.Id).First(&state).Error; err == nil && shouldSkipUnsupportedRatioRetry(&state, common.GetTimestamp()) {
				continue
			}
			if _, err := syncSub2APIRatio(channel, true); err != nil {
				common.SysError(common.LogText("channel upstream ratio sync could not persist its result: channel_id=%d", channel.Id))
			}
		}
	}
}

func shouldSkipUnsupportedRatioRetry(state *model.ChannelCostRatioSyncStatus, now int64) bool {
	return state != nil && state.Status == "unsupported" && now-state.LastAttemptAt < int64((4*time.Hour)/time.Second)
}

type upstreamRatioIDsRequest struct {
	IDs []int `json:"ids"`
}

type upstreamRatioBatchProbeItem struct {
	ChannelID              int      `json:"channel_id"`
	Name                   string   `json:"name"`
	Type                   int      `json:"type"`
	Status                 string   `json:"status"`
	ResolvedRateMultiplier *float64 `json:"resolved_rate_multiplier"`
	GroupRateMultiplier    *float64 `json:"group_rate_multiplier"`
	UserRateMultiplier     *float64 `json:"user_rate_multiplier"`
	CurrentRatio           *float64 `json:"current_ratio"`
	SyncEnabled            bool     `json:"sync_enabled"`
	Error                  string   `json:"error"`
}

type upstreamRatioBatchSyncItem struct {
	ChannelID int                               `json:"channel_id"`
	Sync      *model.ChannelCostRatioSyncResult `json:"sync"`
	Error     string                            `json:"error"`
}

type upstreamRatioBatchSummary struct {
	Enabled     int `json:"enabled"`
	Ineligible  int `json:"ineligible"`
	Success     int `json:"success"`
	Unchanged   int `json:"unchanged"`
	OutOfRange  int `json:"out_of_range"`
	Unsupported int `json:"unsupported"`
	Failed      int `json:"failed"`
}

func parseUpstreamRatioIDs(c *gin.Context) ([]int, error) {
	var request upstreamRatioIDsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return nil, err
	}
	if len(request.IDs) == 0 || len(request.IDs) > 100 {
		return nil, common.NewMessage("Channel ids must contain between 1 and 100 ids")
	}
	ids := make([]int, 0, len(request.IDs))
	seen := make(map[int]struct{}, len(request.IDs))
	for _, id := range request.IDs {
		if id <= 0 {
			return nil, common.NewMessage("Channel ids must be positive")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func loadUpstreamRatioChannels(ids []int) (map[int]*model.Channel, error) {
	channels, err := model.GetChannelsByIds(ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int]*model.Channel, len(channels))
	for _, channel := range channels {
		byID[channel.Id] = channel
	}
	return byID, nil
}

func processUpstreamRatioBatch(ctx context.Context, count int, process func(context.Context, int)) {
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(4, count) {
		workers.Go(func() {
			for index := range jobs {
				process(ctx, index)
			}
		})
	}
	for index := range count {
		select {
		case jobs <- index:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return
		}
	}
	close(jobs)
	workers.Wait()
}

func BatchProbeChannelUpstreamCostRatios(c *gin.Context) {
	ids, err := parseUpstreamRatioIDs(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channels, err := loadUpstreamRatioChannels(ids)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]upstreamRatioBatchProbeItem, len(ids))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	processUpstreamRatioBatch(ctx, len(ids), func(ctx context.Context, index int) {
		id := ids[index]
		item := upstreamRatioBatchProbeItem{ChannelID: id, Status: "not_found"}
		channel := channels[id]
		if channel != nil {
			item.Name, item.Type = channel.Name, channel.Type
			setting := channel.GetSetting()
			item.CurrentRatio, item.SyncEnabled = setting.UpstreamCostRatio, setting.UpstreamCostRatioSyncEnabled
			if eligible, reason := model.UpstreamRatioSyncEligibility(channel); !eligible {
				item.Status, item.Error = "ineligible", reason
			} else if probe, probeErr := probeSub2APIRatioContext(ctx, channel); probeErr != nil {
				item.Error = probeErr.Error()
				if errors.Is(probeErr, errSub2APIRatioUnsupported) {
					item.Status = "unsupported"
				} else if ctx.Err() != nil {
					item.Status, item.Error = "failed", "Batch probe timed out"
				} else {
					item.Status = "failed"
				}
			} else {
				item.Status = "supported"
				item.ResolvedRateMultiplier = probe.ResolvedRateMultiplier
				item.GroupRateMultiplier = probe.GroupRateMultiplier
				item.UserRateMultiplier = probe.UserRateMultiplier
			}
		}
		items[index] = item
	})
	for i := range items {
		if items[i].Status == "" {
			items[i] = upstreamRatioBatchProbeItem{ChannelID: ids[i], Status: "failed", Error: "Batch probe timed out"}
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"items": items}})
}

type upstreamRatioBatchEnableRequest struct {
	IDs      []int    `json:"ids"`
	Currency string   `json:"currency"`
	SyncMin  *float64 `json:"sync_min"`
	SyncMax  *float64 `json:"sync_max"`
	SyncNow  bool     `json:"sync_now"`
}

func BatchEnableChannelUpstreamCostRatioSync(c *gin.Context) {
	var request upstreamRatioBatchEnableRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	ids, err := parseUpstreamRatioIDsFrom(request.IDs)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if request.Currency != "" && request.Currency != "USD" && request.Currency != "CNY" && request.Currency != "CUSTOM" {
		common.ApiError(c, common.NewMessage("Invalid upstream cost currency"))
		return
	}
	for _, bound := range []*float64{request.SyncMin, request.SyncMax} {
		if bound != nil && (math.IsNaN(*bound) || math.IsInf(*bound, 0) || *bound < 0 || *bound > dto.MaxUpstreamCostRatio) {
			common.ApiError(c, common.NewMessage("Upstream ratio sync bounds must be between 0 and 100"))
			return
		}
	}
	if request.SyncMin != nil && request.SyncMax != nil && *request.SyncMin > *request.SyncMax {
		common.ApiError(c, common.NewMessage("Upstream ratio sync minimum must not exceed maximum"))
		return
	}
	channels, err := loadUpstreamRatioChannels(ids)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]upstreamRatioBatchEnableItem, len(ids))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	operatorID := c.GetInt("id")
	processUpstreamRatioBatch(ctx, len(ids), func(ctx context.Context, index int) {
		id := ids[index]
		item := upstreamRatioBatchEnableItem{ChannelID: id}
		channel := channels[id]
		if channel == nil {
			item.Error = "Channel does not exist"
			items[index] = item
			return
		}
		if eligible, reason := model.UpstreamRatioSyncEligibility(channel); !eligible {
			item.Error = reason
			items[index] = item
			return
		}
		if err := model.UpdateChannelCostRatioSyncSettings(id, operatorID, request.Currency, request.SyncMin, request.SyncMax, true); err != nil {
			item.Error = err.Error()
			items[index] = item
			return
		}
		item.Enabled = true
		if request.SyncNow {
			updatedChannel, err := model.GetChannelById(id, true)
			if err != nil {
				item.Sync = &model.ChannelCostRatioSyncResult{Status: "failed", Error: err.Error()}
			} else {
				item.Sync = syncAndClassify(ctx, updatedChannel)
			}
		}
		items[index] = item
	})
	for i := range items {
		if items[i].ChannelID == 0 {
			items[i] = upstreamRatioBatchEnableItem{ChannelID: ids[i], Error: "Batch operation timed out"}
		}
	}
	summary := upstreamRatioBatchSummary{}
	for _, item := range items {
		if item.Enabled {
			summary.Enabled++
		}
		if item.Error != "" {
			if strings.HasPrefix(item.Error, "Upstream ratio sync ") || item.Error == "Official provider APIs do not expose upstream ratios" || item.Error == "This channel's credentials cannot query upstream ratios" {
				summary.Ineligible++
			} else {
				summary.Failed++
			}
		}
		if item.Sync != nil {
			switch item.Sync.Status {
			case "success":
				summary.Success++
			case "unchanged":
				summary.Unchanged++
			case "out_of_range":
				summary.OutOfRange++
			case "unsupported":
				summary.Unsupported++
			case "failed":
				summary.Failed++
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"items": items, "summary": summary}})
}

type upstreamRatioBatchEnableItem struct {
	ChannelID int                               `json:"channel_id"`
	Enabled   bool                              `json:"enabled"`
	Sync      *model.ChannelCostRatioSyncResult `json:"sync"`
	Error     string                            `json:"error"`
}

func syncAndClassify(ctx context.Context, channel *model.Channel) *model.ChannelCostRatioSyncResult {
	result, err := syncSub2APIRatioContext(ctx, channel, false)
	if err != nil {
		return &model.ChannelCostRatioSyncResult{Status: "failed", Error: err.Error()}
	}
	return &result
}

func parseUpstreamRatioIDsFrom(rawIDs []int) ([]int, error) {
	if len(rawIDs) == 0 || len(rawIDs) > 100 {
		return nil, common.NewMessage("Channel ids must contain between 1 and 100 ids")
	}
	ids := make([]int, 0, len(rawIDs))
	seen := make(map[int]struct{}, len(rawIDs))
	for _, id := range rawIDs {
		if id <= 0 {
			return nil, common.NewMessage("Channel ids must be positive")
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func BatchDisableChannelUpstreamCostRatioSync(c *gin.Context) {
	ids, err := parseUpstreamRatioIDs(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	updated := 0
	for _, id := range ids {
		changed, err := model.DisableChannelCostRatioSync(id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if changed {
			updated++
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"updated": updated}})
}

func BatchSyncChannelUpstreamCostRatios(c *gin.Context) {
	ids, err := parseUpstreamRatioIDs(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channels, err := loadUpstreamRatioChannels(ids)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]upstreamRatioBatchSyncItem, len(ids))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	processUpstreamRatioBatch(ctx, len(ids), func(ctx context.Context, index int) {
		id := ids[index]
		item := upstreamRatioBatchSyncItem{ChannelID: id}
		channel := channels[id]
		if channel == nil {
			item.Error = "Channel does not exist"
		} else if eligible, reason := model.UpstreamRatioSyncEligibility(channel); !eligible {
			item.Error = reason
		} else {
			item.Sync = syncAndClassify(ctx, channel)
		}
		items[index] = item
	})
	for i := range items {
		if items[i].ChannelID == 0 {
			items[i] = upstreamRatioBatchSyncItem{ChannelID: ids[i], Sync: &model.ChannelCostRatioSyncResult{Status: "failed", Error: "Batch operation timed out"}}
		}
	}
	summary := upstreamRatioBatchSummary{}
	for _, item := range items {
		if item.Error != "" {
			if item.Error == "Official provider APIs do not expose upstream ratios" || item.Error == "This channel's credentials cannot query upstream ratios" || strings.HasPrefix(item.Error, "Upstream ratio sync ") {
				summary.Ineligible++
			} else {
				summary.Failed++
			}
		}
		if item.Sync != nil {
			switch item.Sync.Status {
			case "success":
				summary.Success++
			case "unchanged":
				summary.Unchanged++
			case "out_of_range":
				summary.OutOfRange++
			case "unsupported":
				summary.Unsupported++
			case "failed":
				summary.Failed++
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"items": items, "summary": summary}})
}
