package productsubscription

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sourcegraph/log/logtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sourcegraph/sourcegraph/cmd/frontend/graphqlbackend"
	"github.com/sourcegraph/sourcegraph/enterprise/internal/license"
	"github.com/sourcegraph/sourcegraph/enterprise/internal/licensing"
	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/database/dbtest"
	"github.com/sourcegraph/sourcegraph/internal/timeutil"
)

func TestLLMProxyAccessResolverRateLimit(t *testing.T) {
	logger := logtest.Scoped(t)
	db := database.NewDB(logger, dbtest.NewDB(logger, t))
	ctx := context.Background()

	u, err := db.Users().Create(ctx, database.NewUser{Username: "u"})
	require.NoError(t, err)

	subID, err := dbSubscriptions{db: db}.Create(ctx, u.ID, "")
	require.NoError(t, err)
	info := license.Info{
		Tags:      []string{fmt.Sprintf("plan:%s", licensing.PlanEnterprise1)},
		UserCount: 10,
		ExpiresAt: timeutil.Now().Add(time.Minute),
	}
	_, err = dbLicenses{db: db}.Create(ctx, subID, "k2", 1, info)
	require.NoError(t, err)

	// Enable access to LLM proxy.
	tru := true
	err = dbSubscriptions{db: db}.Update(ctx, subID, dbSubscriptionUpdate{llmProxyAccess: &graphqlbackend.UpdateLLMProxyAccessInput{Enabled: &tru}})
	require.NoError(t, err)

	t.Run("default rate limit for a plan", func(t *testing.T) {
		sub, err := dbSubscriptions{db: db}.GetByID(ctx, subID)
		require.NoError(t, err)

		r := llmProxyAccessResolver{sub: &productSubscription{v: sub, db: db}}
		wantRateLimit := licensing.NewLLMProxyChatRateLimit(licensing.PlanEnterprise1, pointify(int(info.UserCount)), []string{})
		rateLimit, err := r.ChatCompletionsRateLimit(ctx)
		require.NoError(t, err)

		assert.Equal(t, wantRateLimit.Limit, rateLimit.Limit())
		assert.Equal(t, wantRateLimit.IntervalSeconds, rateLimit.IntervalSeconds())
	})

	t.Run("override default rate limit for a plan", func(t *testing.T) {
		err := (dbSubscriptions{db: db}.Update(ctx, subID, dbSubscriptionUpdate{
			llmProxyAccess: &graphqlbackend.UpdateLLMProxyAccessInput{
				ChatCompletionsRateLimit: pointify(int32(10)),
			},
		}))
		require.NoError(t, err)

		sub, err := dbSubscriptions{db: db}.GetByID(ctx, subID)
		require.NoError(t, err)

		r := llmProxyAccessResolver{sub: &productSubscription{v: sub, db: db}}
		defaultRateLimit := licensing.NewLLMProxyChatRateLimit(licensing.PlanEnterprise1, pointify(10), []string{})
		rateLimit, err := r.ChatCompletionsRateLimit(ctx)
		require.NoError(t, err)

		assert.Equal(t, int32(10), rateLimit.Limit())
		assert.Equal(t, defaultRateLimit.IntervalSeconds, rateLimit.IntervalSeconds())
	})
}
