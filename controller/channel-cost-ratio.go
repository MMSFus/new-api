package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
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

func probeSub2APIRatio(channel *model.Channel) (*sub2APIRatioProbe, error) {
	if channel.Type != constant.ChannelTypeSub2API {
		return nil, common.NewMessage(model.OnlySub2APIRatioSync)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(channel.GetBaseURL()), "/")
	if baseURL == "" {
		return nil, common.NewMessage("Sub2API channel is missing base_url")
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
	body, err := GetResponseBody(http.MethodGet, baseURL+"/v1/sub2api/billing", channel, GetAuthHeader(key), responseBodyLimits{Timeout: 10 * time.Second, MaxBytes: 64 << 10})
	if err != nil {
		return nil, sanitizeAdvancedCustomRequestError(err, key, baseURL+"/v1/sub2api/billing")
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
	if channel.Type != constant.ChannelTypeSub2API {
		return model.ChannelCostRatioSyncResult{}, common.NewMessage(model.OnlySub2APIRatioSync)
	}
	probe, err := probeSub2APIRatio(channel)
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
	return model.ApplyChannelCostRatioSync(channel.Id, value, status, message, info, automatic)
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
	success := result.Status != "failed" && result.Status != "unsupported"
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
			if channel.Type != constant.ChannelTypeSub2API || !channel.GetSetting().UpstreamCostRatioSyncEnabled {
				continue
			}
			if _, err := syncSub2APIRatio(channel, true); err != nil {
				common.SysError(common.LogText("channel upstream ratio sync could not persist its result: channel_id=%d", channel.Id))
			}
		}
	}
}
