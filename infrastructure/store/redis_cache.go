package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/open-strata-ai/ai-tool-registry/domain"
	"github.com/redis/go-redis/v9"
)

// RedisCache provides an L2 hot-copy of tools and skills using Redis hashes.
type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisCache opens a Redis connection for caching tool definitions.
func NewRedisCache(addr string) *RedisCache {
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	return &RedisCache{client: rdb, ttl: 5 * time.Minute}
}

const cachePrefix = "toolcache:"

func (c *RedisCache) cacheKey(tenant, name, version string) string {
	return cachePrefix + tenant + ":" + name + ":" + version
}

func (c *RedisCache) SetTool(def domain.ToolDefinition) error {
	data, _ := json.Marshal(def)
	return c.client.Set(context.Background(), c.cacheKey(def.TenantID, def.Name, def.Version), data, c.ttl).Err()
}

func (c *RedisCache) GetTool(tenant, name, version string) (domain.ToolDefinition, bool) {
	data, err := c.client.Get(context.Background(), c.cacheKey(tenant, name, version)).Bytes()
	if err != nil {
		return domain.ToolDefinition{}, false
	}
	var def domain.ToolDefinition
	json.Unmarshal(data, &def)
	return def, true
}

func (c *RedisCache) DeleteTool(tenant, name string) error {
	// Delete all versions by scanning
	iter := c.client.Scan(context.Background(), 0, cachePrefix+tenant+":"+name+":*", 100).Iterator()
	for iter.Next(context.Background()) {
		c.client.Del(context.Background(), iter.Val())
	}
	return nil
}
