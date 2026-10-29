package model

import (
	"cmp"
	"slices"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// channelCostLogKey is the root_info key of a consume or refund log's cost
// snapshot. Root sees it in log details; the cost report reads it for anyone
// granted authz.ChannelCostRead.
const channelCostLogKey = "channel_cost"

// ChannelCostSnapshot freezes the channel's upstream cost of one log, so later
// changes to the channel's upstream_cost_ratio do not rewrite history.
type ChannelCostSnapshot struct {
	UpstreamRatio float64 `json:"upstream_ratio"`
	GroupRatio    float64 `json:"group_ratio"`
	// BaseQuota is the quota at the official price: what the log charged (or,
	// under the simulated cache, what the upstream usage costs) divided by the
	// group ratio. Surcharges added after the group ratio, such as tiered tool
	// call fees, make it an estimate.
	BaseQuota float64 `json:"base_quota"`
	CostQuota float64 `json:"cost_quota"`
}

// SetCostBasisQuota overrides the charged quota as the basis of the channel
// cost, for logs whose charge does not follow the upstream usage.
func (o *LogOther) SetCostBasisQuota(quota int) {
	if o == nil {
		return
	}
	o.costBasisQuota = &quota
}

// attachChannelCost snapshots the cost of a log that charged quota on the
// channel. A missing group ratio (free groups, logs not produced by a relay)
// or a channel without upstream_cost_ratio leaves the log uncosted.
func attachChannelCost(other *LogOther, channelId int, quota int, groupRatio float64) {
	if other == nil || channelId == 0 || !(groupRatio > 0) {
		return
	}
	channel, err := CacheGetChannel(channelId)
	if err != nil {
		return
	}
	ratio := channel.GetSetting().UpstreamCostRatio
	if ratio == nil {
		return
	}
	basis := quota
	if other.costBasisQuota != nil {
		basis = *other.costBasisQuota
	}
	baseQuota := float64(basis) / groupRatio
	other.SetRoot(channelCostLogKey, ChannelCostSnapshot{
		UpstreamRatio: *ratio,
		GroupRatio:    groupRatio,
		BaseQuota:     baseQuota,
		CostQuota:     baseQuota * *ratio,
	})
}

// ChannelCostAmount sums revenue and cost in quota units. Refunds count
// negatively. Revenue splits into costed and uncosted parts, so profit and
// margin are computed only over the logs whose cost is known.
type ChannelCostAmount struct {
	Requests         int64   `json:"requests"`
	Refunds          int64   `json:"refunds"`
	UncostedRequests int64   `json:"uncosted_requests"`
	Revenue          int64   `json:"revenue"`
	CostedRevenue    int64   `json:"costed_revenue"`
	Cost             float64 `json:"cost"`
	Profit           float64 `json:"profit"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	// SimulatedCacheDelta is the charged quota minus what the upstream usage
	// would have charged, over logs the simulated prompt cache rewrote.
	SimulatedCacheDelta int64 `json:"simulated_cache_delta"`
}

func (a *ChannelCostAmount) add(row channelCostRow) {
	sign := int64(1)
	if row.Type == LogTypeRefund {
		sign = -1
		a.Refunds++
	} else {
		a.Requests++
		a.PromptTokens += int64(row.PromptTokens)
		a.CompletionTokens += int64(row.CompletionTokens)
	}
	quota := sign * int64(row.Quota)
	a.Revenue += quota
	cost := gjson.Get(row.Other, "root_info."+channelCostLogKey+".cost_quota")
	if cost.Exists() {
		a.CostedRevenue += quota
		a.Cost += float64(sign) * cost.Float()
	} else if row.Type != LogTypeRefund {
		a.UncostedRequests++
	}
	a.SimulatedCacheDelta += gjson.Get(row.Other, "admin_info.simulated_cache.quota_delta").Int()
	a.Profit = float64(a.CostedRevenue) - a.Cost
}

type ChannelCostChannel struct {
	ChannelCostAmount
	ChannelId   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	// UpstreamRatio is the channel's current setting; nil when unset or when
	// the channel no longer exists.
	UpstreamRatio *float64 `json:"upstream_ratio"`
}

type ChannelCostModel struct {
	ChannelCostAmount
	ModelName string `json:"model_name"`
}

type ChannelCostGroup struct {
	ChannelCostAmount
	Group string `json:"group"`
}

type ChannelCostUser struct {
	ChannelCostAmount
	UserId   int    `json:"user_id"`
	Username string `json:"username"`
}

type ChannelCostBucket struct {
	ChannelCostAmount
	// Time is the start of the hour, in Unix seconds. The console groups
	// hours into days in the viewer's time zone.
	Time int64 `json:"time"`
}

// channelCostRankLimit bounds the model and user rankings; channels and
// groups are few enough to return in full.
const channelCostRankLimit = 100

type ChannelCostReport struct {
	Total      ChannelCostAmount    `json:"total"`
	Trend      []ChannelCostBucket  `json:"trend"`
	Channels   []ChannelCostChannel `json:"channels"`
	Models     []ChannelCostModel   `json:"models"`
	ModelCount int                  `json:"model_count"`
	Groups     []ChannelCostGroup   `json:"groups"`
	Users      []ChannelCostUser    `json:"users"`
	UserCount  int                  `json:"user_count"`
}

type channelCostRow struct {
	CreatedAt        int64
	Type             int
	UserId           int
	Username         string
	ModelName        string
	ChannelId        int
	Group            string `gorm:"column:group"`
	Quota            int
	PromptTokens     int
	CompletionTokens int
	Other            string
}

// channelCostLogTypes are the logs that move revenue: charges and refunds.
var channelCostLogTypes = []int{LogTypeConsume, LogTypeRefund}

// rankByRevenue sorts rows by revenue, highest first, and keeps the first
// limit (all when limit is 0).
func rankByRevenue[T any](rows []T, amount func(*T) *ChannelCostAmount, limit int) []T {
	slices.SortFunc(rows, func(a, b T) int {
		return cmp.Compare(amount(&b).Revenue, amount(&a).Revenue)
	})
	if limit > 0 && len(rows) > limit {
		return rows[:limit]
	}
	return rows
}

// SumChannelCost streams the consume and refund logs of a (time-bounded)
// search once and aggregates them by hour, channel, model, group and user.
func SumChannelCost(filter LogFilter) (report ChannelCostReport, err error) {
	filter.Type = LogTypeUnknown
	filter.Types = channelCostLogTypes
	tx, err := filter.apply(LOG_DB)
	if err != nil {
		return report, err
	}
	rows, err := tx.Model(&Log{}).Select("logs.created_at, logs.type, logs.user_id, logs.username, logs.model_name, " +
		"logs.channel_id, logs." + logGroupCol + ", logs.quota, logs.prompt_tokens, logs.completion_tokens, logs.other").Rows()
	if err != nil {
		return report, err
	}
	defer rows.Close()

	trend := map[int64]*ChannelCostBucket{}
	channels := map[int]*ChannelCostChannel{}
	models := map[string]*ChannelCostModel{}
	groups := map[string]*ChannelCostGroup{}
	users := map[int]*ChannelCostUser{}
	for rows.Next() {
		var row channelCostRow
		if err = LOG_DB.ScanRows(rows, &row); err != nil {
			return report, err
		}
		report.Total.add(row)

		hour := row.CreatedAt - row.CreatedAt%3600
		if trend[hour] == nil {
			trend[hour] = &ChannelCostBucket{Time: hour}
		}
		trend[hour].add(row)
		if channels[row.ChannelId] == nil {
			channels[row.ChannelId] = &ChannelCostChannel{ChannelId: row.ChannelId}
		}
		channels[row.ChannelId].add(row)
		if models[row.ModelName] == nil {
			models[row.ModelName] = &ChannelCostModel{ModelName: row.ModelName}
		}
		models[row.ModelName].add(row)
		if groups[row.Group] == nil {
			groups[row.Group] = &ChannelCostGroup{Group: row.Group}
		}
		groups[row.Group].add(row)
		if users[row.UserId] == nil {
			users[row.UserId] = &ChannelCostUser{UserId: row.UserId, Username: row.Username}
		}
		users[row.UserId].add(row)
	}
	if err = rows.Err(); err != nil {
		return report, err
	}

	for _, bucket := range trend {
		report.Trend = append(report.Trend, *bucket)
	}
	slices.SortFunc(report.Trend, func(a, b ChannelCostBucket) int { return cmp.Compare(a.Time, b.Time) })
	for _, channel := range channels {
		if cached, err := CacheGetChannel(channel.ChannelId); err == nil {
			channel.ChannelName = cached.Name
			channel.UpstreamRatio = cached.GetSetting().UpstreamCostRatio
		}
		report.Channels = append(report.Channels, *channel)
	}
	report.Channels = rankByRevenue(report.Channels, func(c *ChannelCostChannel) *ChannelCostAmount { return &c.ChannelCostAmount }, 0)
	for _, model := range models {
		report.Models = append(report.Models, *model)
	}
	report.ModelCount = len(report.Models)
	report.Models = rankByRevenue(report.Models, func(m *ChannelCostModel) *ChannelCostAmount { return &m.ChannelCostAmount }, channelCostRankLimit)
	for _, group := range groups {
		report.Groups = append(report.Groups, *group)
	}
	report.Groups = rankByRevenue(report.Groups, func(g *ChannelCostGroup) *ChannelCostAmount { return &g.ChannelCostAmount }, 0)
	for _, user := range users {
		report.Users = append(report.Users, *user)
	}
	report.UserCount = len(report.Users)
	report.Users = rankByRevenue(report.Users, func(u *ChannelCostUser) *ChannelCostAmount { return &u.ChannelCostAmount }, channelCostRankLimit)
	return report, nil
}

// ChannelCostLog is one consume or refund log as the cost report lists it.
// It carries no request content, only the numbers behind the report.
type ChannelCostLog struct {
	Id                  int                  `json:"id"`
	CreatedAt           int64                `json:"created_at"`
	Type                int                  `json:"type"`
	UserId              int                  `json:"user_id"`
	Username            string               `json:"username"`
	ModelName           string               `json:"model_name"`
	ChannelId           int                  `json:"channel_id"`
	ChannelName         string               `json:"channel_name"`
	Group               string               `json:"group"`
	Quota               int                  `json:"quota"`
	PromptTokens        int                  `json:"prompt_tokens"`
	CompletionTokens    int                  `json:"completion_tokens"`
	RequestId           string               `json:"request_id"`
	Cost                *ChannelCostSnapshot `json:"cost"`
	SimulatedCacheDelta *int64               `json:"simulated_cache_delta"`
}

// GetChannelCostLogs pages the consume and refund logs of a search with
// their cost snapshots.
func GetChannelCostLogs(filter LogFilter, startIdx int, num int) ([]ChannelCostLog, int64, error) {
	filter.Type = LogTypeUnknown
	filter.Types = channelCostLogTypes
	logs, total, err := GetAllLogs(filter, startIdx, num)
	if err != nil {
		return nil, 0, err
	}
	items := make([]ChannelCostLog, 0, len(logs))
	for _, log := range logs {
		item := ChannelCostLog{
			Id:               log.Id,
			CreatedAt:        log.CreatedAt,
			Type:             log.Type,
			UserId:           log.UserId,
			Username:         log.Username,
			ModelName:        log.ModelName,
			ChannelId:        log.ChannelId,
			ChannelName:      log.ChannelName,
			Group:            log.Group,
			Quota:            log.Quota,
			PromptTokens:     log.PromptTokens,
			CompletionTokens: log.CompletionTokens,
			RequestId:        log.RequestId,
		}
		if snapshot := gjson.Get(log.Other, "root_info."+channelCostLogKey); snapshot.IsObject() {
			item.Cost = &ChannelCostSnapshot{
				UpstreamRatio: snapshot.Get("upstream_ratio").Float(),
				GroupRatio:    snapshot.Get("group_ratio").Float(),
				BaseQuota:     snapshot.Get("base_quota").Float(),
				CostQuota:     snapshot.Get("cost_quota").Float(),
			}
		}
		if delta := gjson.Get(log.Other, "admin_info.simulated_cache.quota_delta"); delta.Exists() {
			value := delta.Int()
			item.SimulatedCacheDelta = &value
		}
		items = append(items, item)
	}
	return items, total, nil
}

// HideSimulatedCacheAudit removes admin_info.simulated_cache, for admins
// without authz.SimulatedCacheRead.
func (log *Log) HideSimulatedCacheAudit() {
	if !strings.Contains(log.Other, `"simulated_cache"`) {
		return
	}
	if other, err := sjson.Delete(log.Other, "admin_info.simulated_cache"); err == nil {
		log.Other = other
	}
}
