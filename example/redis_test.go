package example_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/worldline-go/cache"
	"github.com/worldline-go/cache/store/redis"
	"github.com/worldline-go/test/container"
)

type RedisSuite struct {
	suite.Suite
	container *container.RedisContainer
}

func (s *RedisSuite) SetupSuite() {
	s.container = container.Redis(s.T())
}

func (s *RedisSuite) TearDownSuite() {
	s.container.Stop(s.T())
}

func TestExampleTestSuiteRedis(t *testing.T) {
	suite.Run(t, new(RedisSuite))
}

func (s *RedisSuite) TestCache() {
	redisClient, err := redis.New(redis.Connection{
		Addr: s.container.Address(),
	})
	if err != nil {
		s.T().Fatal(err)
	}

	c, err := cache.New(s.T().Context(),
		redis.Store(redisClient),
		cache.WithStoreConfig(redis.Config{
			TTL: 3 * time.Second,
		}),
	)
	if err != nil {
		s.T().Fatal(err)
	}

	err = c.Set(s.T().Context(), "key", "value")
	require.NoError(s.T(), err)

	v, ok, err := c.Get(s.T().Context(), "key")
	require.NoError(s.T(), err)
	require.True(s.T(), ok)
	require.Equal(s.T(), "value", v)

	err = c.Delete(s.T().Context(), "key")
	require.NoError(s.T(), err)

	v, ok, err = c.Get(s.T().Context(), "key")
	require.NoError(s.T(), err)
	require.False(s.T(), ok)
	require.Empty(s.T(), v)

	err = c.Set(s.T().Context(), "test", "timeout")
	require.NoError(s.T(), err)

	time.Sleep(4 * time.Second)

	v, ok, err = c.Get(s.T().Context(), "test")
	require.NoError(s.T(), err)
	require.False(s.T(), ok)
	require.Empty(s.T(), v)
}
