package main

import (
	"fmt"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/response-cache/config"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/log"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/wrapper"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/tidwall/resp"
)

// CheckCacheForKey checks if the key is in the cache
func CheckCacheForKey(key string, ctx wrapper.HttpContext, c config.PluginConfig) error {
	activeCacheProvider := c.GetCacheProvider()
	if activeCacheProvider == nil {
		return fmt.Errorf("[CheckCacheForKey] no cache provider configured")
	}

	queryKey := activeCacheProvider.GetCacheKeyPrefix() + key
	log.Debugf("[CheckCacheForKey] querying cache with key: %s", queryKey)

	err := activeCacheProvider.Get(queryKey, func(response resp.Value) {
		handleCacheResponse(key, response, ctx, c)
	})

	if err != nil {
		log.Errorf("[CheckCacheForKey] failed to retrieve key: %s from cache, error: %v", key, err)
		return err
	}

	return nil
}

// handleCacheResponse processes cache response and handles cache hits and misses.
func handleCacheResponse(key string, response resp.Value, ctx wrapper.HttpContext, c config.PluginConfig) {
	if err := response.Error(); err == nil && !response.IsNull() {
		log.Infof("[handleCacheResponse] cache hit for key: %s", key)
		processCacheHit(key, response.String(), ctx, c)
		return
	}

	log.Infof("[handleCacheResponse] cache miss for key: %s", key)
	if err := response.Error(); err != nil {
		log.Errorf("[handleCacheResponse] error retrieving key: %s from cache, error: %v", key, err)
	}
	proxywasm.ResumeHttpRequest()
}

// processCacheHit handles a successful cache hit.
func processCacheHit(key string, response string, ctx wrapper.HttpContext, c config.PluginConfig) {
	if strings.TrimSpace(response) == "" {
		log.Warnf("[processCacheHit] cached response for key %s is empty", key)
		proxywasm.ResumeHttpRequest()
		return
	}

	log.Debugf("[processCacheHit] cached response for key %s: %s", key, response)

	ctx.SetContext(CACHE_KEY_CONTEXT_KEY, nil)

	contentType := fmt.Sprintf("%s", c.CacheValueFromBodyType)
	body := response

	if c.CacheValueFromBodyType == "original" {
		//Split the response into content type and body
		parts := strings.SplitN(response, ":", 2)
		if len(parts) == 2 {
			contentType = parts[0]
			body = parts[1]
		} else {
			log.Warnf("[processCacheHit] Invalid cache value when CacheValueFromBodyType=original key:%s value:%s", key, response)
			proxywasm.ResumeHttpRequest()
			return
		}
	}

	headers := [][2]string{
		{"content-type", contentType},
		{"x-cache-status", "hit"},
	}

	proxywasm.SendHttpResponseWithDetail(200, "response-cache.hit", headers, []byte(body), -1)

}

// Caches the response value
func cacheResponse(ctx wrapper.HttpContext, c config.PluginConfig, key string, value string) {
	if strings.TrimSpace(value) == "" {
		log.Warnf("[cacheResponse] cached value for key %s is empty", key)
		return
	}

	activeCacheProvider := c.GetCacheProvider()
	if activeCacheProvider != nil {
		queryKey := activeCacheProvider.GetCacheKeyPrefix() + key
		_ = activeCacheProvider.Set(queryKey, value, nil)
		log.Debugf("[cacheResponse] cache set success, key: %s, length of value: %d", queryKey, len(value))
	}
}
