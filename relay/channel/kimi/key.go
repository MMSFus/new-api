package kimi

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// KeyConfig 为 Kimi 渠道的密钥配置。渠道 Key 栏填 JSON：
//
//	{"refresh_token": "eyJ...", "region": "global"}
//
// region 取值 "global"（kimi.ai，默认）或 "cn"（kimi.com）。
// 为便于手工录入，Key 栏也允许直接填 refresh_token 原文（等价于 global）。
type KeyConfig struct {
	RefreshToken string `json:"refresh_token"`
	Region       string `json:"region,omitempty"`
}

func ParseKey(raw string) (*KeyConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("kimi channel: empty key")
	}
	if !strings.HasPrefix(raw, "{") {
		return &KeyConfig{RefreshToken: raw, Region: RegionGlobal}, nil
	}
	var key KeyConfig
	if err := common.Unmarshal([]byte(raw), &key); err != nil {
		return nil, errors.New("kimi channel: invalid key json")
	}
	key.RefreshToken = strings.TrimSpace(key.RefreshToken)
	if key.RefreshToken == "" {
		return nil, errors.New("kimi channel: refresh_token is required")
	}
	return &key, nil
}

const (
	RegionGlobal = "global"
	RegionCN     = "cn"
)

// Hosts 为一个区域部署的站点主机与鉴权主机。
type Hosts struct {
	Site string // 网页站点，如 https://www.kimi.ai
	Auth string // 刷新 token 的鉴权主机，如 https://auth.kimi.ai
}

var regionHosts = map[string]Hosts{
	RegionGlobal: {Site: "https://www.kimi.ai", Auth: "https://auth.kimi.ai"},
	RegionCN:     {Site: "https://www.kimi.com", Auth: "https://auth.kimi.com"},
}

// ResolveHosts 依据 region 与渠道 Base URL 决定上游主机。
// Base URL 非空时覆盖站点主机（可用于反代域名），鉴权主机仍按 region 取默认值，
// 除非 Base URL 指向 www.kimi.com / www.kimi.ai 之外的自定义主机。
func (k *KeyConfig) ResolveHosts(channelBaseURL string) Hosts {
	h, ok := regionHosts[k.Region]
	if !ok {
		h = regionHosts[RegionGlobal]
	}
	base := strings.TrimRight(strings.TrimSpace(channelBaseURL), "/")
	if base == "" {
		return h
	}
	h.Site = base
	return h
}
