package controller

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestBoundLogFilterRangeRejectsInvalidStartBeforeSubtracting(t *testing.T) {
	for _, start := range []int64{-1, math.MinInt64} {
		filter := model.LogFilter{StartTimestamp: start}
		require.Error(t, boundLogFilterRange(&filter, channelCostMaxRange))
	}
	filter := model.LogFilter{StartTimestamp: 100, EndTimestamp: 99}
	require.Error(t, boundLogFilterRange(&filter, channelCostMaxRange))
}
