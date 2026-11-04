package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectAntigravityCredentialSkipsDisabledCredentials(t *testing.T) {
	channel := &model.Channel{
		Key: "invalid-disabled-credential\n{\"type\":\"antigravity\",\"access_token\":\"enabled-token\",\"project_id\":\"test-project\"}",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:         true,
			MultiKeyMode:       constant.MultiKeyModeRandom,
			MultiKeyStatusList: map[int]int{0: common.ChannelStatusAutoDisabled},
		},
	}
	selected, err := selectAntigravityCredential(channel)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"antigravity","access_token":"enabled-token","project_id":"test-project"}`, selected)

	channel.ChannelInfo.MultiKeyStatusList[1] = common.ChannelStatusAutoDisabled
	_, err = selectAntigravityCredential(channel)
	require.ErrorContains(t, err, "no enabled keys")
}
