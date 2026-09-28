package setting

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withRateLimitTables(t *testing.T, legacy, global, private string) {
	t.Helper()
	ModelRequestRateLimitMutex.RLock()
	prevLegacy, prevGlobal, prevPrivate := ModelRequestRateLimitGroup, ModelRequestRateLimitGlobalGroup, ModelRequestRateLimitPrivateGroup
	ModelRequestRateLimitMutex.RUnlock()
	prevDuration, prevCount, prevSuccess := ModelRequestRateLimitDurationMinutes, ModelRequestRateLimitCount, ModelRequestRateLimitSuccessCount
	t.Cleanup(func() {
		ModelRequestRateLimitMutex.Lock()
		ModelRequestRateLimitGroup, ModelRequestRateLimitGlobalGroup, ModelRequestRateLimitPrivateGroup = prevLegacy, prevGlobal, prevPrivate
		ModelRequestRateLimitMutex.Unlock()
		ModelRequestRateLimitDurationMinutes, ModelRequestRateLimitCount, ModelRequestRateLimitSuccessCount = prevDuration, prevCount, prevSuccess
	})
	ModelRequestRateLimitDurationMinutes, ModelRequestRateLimitCount, ModelRequestRateLimitSuccessCount = 1, 7, 70
	require.NoError(t, UpdateModelRequestRateLimitGroupByJSONString(legacy))
	require.NoError(t, UpdateModelRequestRateLimitGlobalGroupByJSONString(global))
	require.NoError(t, UpdateModelRequestRateLimitPrivateGroupByJSONString(private))
}

func TestResolveModelRequestRateLimitPriority(t *testing.T) {
	withRateLimitTables(t,
		`{"vip":[5,50],"default":[6,60]}`,
		`{"claude":{"total":3,"success":30,"duration":5},"gpt":{"total":4,"success":40}}`,
		`{"vip":{"claude":{"total":1,"success":10},"*":{"total":2,"success":20,"duration":10}},"svip":{"claude":{"total":9,"success":90}}}`,
	)

	cases := []struct {
		name                      string
		userGroup, called, legacy string
		total, success, duration  int
		scope, source             string
	}{
		{"private exact beats everything", "vip", "claude", "vip", 1, 10, 1, "claude", ModelRateLimitSourcePrivate},
		{"private wildcard beats global", "vip", "gpt", "vip", 2, 20, 10, ModelRateLimitAnyGroup, ModelRateLimitSourcePrivateWildcard},
		{"global applies to any user", "default", "claude", "default", 3, 30, 5, "claude", ModelRateLimitSourceGlobal},
		{"global without duration uses default window", "svip", "gpt", "svip", 4, 40, 1, "gpt", ModelRateLimitSourceGlobal},
		{"legacy table keyed by legacy group", "default", "other", "default", 6, 60, 1, "", ModelRateLimitSourceLegacy},
		{"legacy follows token group", "default", "vip", "vip", 5, 50, 1, "", ModelRateLimitSourceLegacy},
		{"default when nothing matches", "free", "other", "free", 7, 70, 1, "", ModelRateLimitSourceDefault},
		{"empty user group skips private", "", "claude", "", 3, 30, 5, "claude", ModelRateLimitSourceGlobal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule := ResolveModelRequestRateLimit(tc.userGroup, tc.called, tc.legacy)
			assert.Equal(t, ModelRateLimitRule{Total: tc.total, Success: tc.success, DurationMinutes: tc.duration, Scope: tc.scope, Source: tc.source}, rule)
		})
	}
}

func TestModelRequestRateLimitJSONRoundTrip(t *testing.T) {
	withRateLimitTables(t, `{"vip":[5,50]}`, `{"claude":{"total":3,"success":30}}`, `{"vip":{"*":{"total":2,"success":20,"duration":10}}}`)
	assert.JSONEq(t, `{"vip":[5,50]}`, ModelRequestRateLimitGroup2JSONString())
	assert.JSONEq(t, `{"claude":{"total":3,"success":30}}`, ModelRequestRateLimitGlobalGroup2JSONString())
	assert.JSONEq(t, `{"vip":{"*":{"total":2,"success":20,"duration":10}}}`, ModelRequestRateLimitPrivateGroup2JSONString())

	// A rejected value must not wipe the table in use.
	require.Error(t, UpdateModelRequestRateLimitGlobalGroupByJSONString(`{bad`))
	assert.JSONEq(t, `{"claude":{"total":3,"success":30}}`, ModelRequestRateLimitGlobalGroup2JSONString())
	require.NoError(t, UpdateModelRequestRateLimitGlobalGroupByJSONString(""))
	assert.JSONEq(t, `{}`, ModelRequestRateLimitGlobalGroup2JSONString())
}

func TestCheckModelRequestRateLimitRules(t *testing.T) {
	assert.NoError(t, CheckModelRequestRateLimitGlobalGroup(`{"claude":{"total":0,"success":0,"duration":1440}}`))
	assert.Error(t, CheckModelRequestRateLimitGlobalGroup(`{"claude":{"total":-1,"success":1}}`))
	assert.Error(t, CheckModelRequestRateLimitGlobalGroup(`{"claude":{"total":1,"success":1,"duration":1441}}`))
	assert.Error(t, CheckModelRequestRateLimitGlobalGroup(`{"*":{"total":1,"success":1}}`), "wildcard is private-only")
	assert.Error(t, CheckModelRequestRateLimitGlobalGroup(`{" claude":{"total":1,"success":1}}`))
	assert.Error(t, CheckModelRequestRateLimitGlobalGroup(`{"claude":[1,2]}`))

	assert.NoError(t, CheckModelRequestRateLimitPrivateGroup(`{"vip":{"claude":{"total":1,"success":1},"*":{"total":2,"success":2}}}`))
	assert.Error(t, CheckModelRequestRateLimitPrivateGroup(`{"*":{"claude":{"total":1,"success":1}}}`))
	assert.Error(t, CheckModelRequestRateLimitPrivateGroup(`{"vip":{"claude":{"total":1,"success":-2}}}`))
	assert.Error(t, CheckModelRequestRateLimitPrivateGroup(`{"vip":{"claude":{"total":9223372036854775807,"success":1}}}`))

	assert.NoError(t, CheckModelRequestRateLimitGroup(`{"vip":[0,1]}`))
	assert.Error(t, CheckModelRequestRateLimitGroup(`{"vip":[0,0]}`))
}

// TestModelRequestRateLimitUpdatesTakeWriteLock fails under -race when an
// update writes the tables while holding only the read lock.
func TestModelRequestRateLimitUpdatesTakeWriteLock(t *testing.T) {
	withRateLimitTables(t, `{}`, `{}`, `{}`)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = UpdateModelRequestRateLimitGroupByJSONString(`{"vip":[1,2]}`)
				_ = UpdateModelRequestRateLimitGlobalGroupByJSONString(`{"claude":{"total":1,"success":2}}`)
				_ = UpdateModelRequestRateLimitPrivateGroupByJSONString(`{"vip":{"*":{"total":1,"success":2}}}`)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_, _, _ = GetGroupRateLimit("vip")
				_ = ResolveModelRequestRateLimit("vip", "claude", "vip")
				_ = ModelRequestRateLimitPrivateGroup2JSONString()
			}
		}()
	}
	wg.Wait()

	// The fixed update takes the write lock: it must block while a reader
	// holds the read lock.
	ModelRequestRateLimitMutex.RLock()
	done := make(chan struct{})
	go func() {
		_ = UpdateModelRequestRateLimitGroupByJSONString(`{"vip":[3,4]}`)
		close(done)
	}()
	select {
	case <-done:
		ModelRequestRateLimitMutex.RUnlock()
		t.Fatal("update completed while a read lock was held")
	case <-time.After(100 * time.Millisecond):
	}
	ModelRequestRateLimitMutex.RUnlock()
	<-done
	total, success, found := GetGroupRateLimit("vip")
	assert.True(t, found)
	assert.Equal(t, [2]int{3, 4}, [2]int{total, success})
}
