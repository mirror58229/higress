// 这个文件中主要将OnHttpRequestHeaders、OnHttpRequestBody、OnHttpResponseHeaders、OnHttpResponseBody这四个函数实现
// 其中的缓存思路调用cache.go中的逻辑
package main

import (
	"strconv"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/response-cache/config"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/log"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/wrapper"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/tidwall/gjson"
)

const (
	PLUGIN_NAME           = "response-cache"
	CACHE_KEY_CONTEXT_KEY = "cacheKey"
	CACHE_VALUE_RESP_TYPE = "cacheResponeseType"
	SKIP_CACHE_HEADER     = "x-higress-skip-response-cache"

	DEFAULT_MAX_BODY_BYTES uint32 = 10 * 1024 * 1024
)

func main() {
	// CreateClient()
	wrapper.SetCtx(
		PLUGIN_NAME,
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
		wrapper.ProcessResponseBody(onHttpResponseBody),
	)
}

func parseConfig(json gjson.Result, c *config.PluginConfig) error {
	c.FromJson(json)
	if err := c.Validate(); err != nil {
		log.Errorf("validate config failed: %v", err)
		return err
	}

	if err := c.Complete(); err != nil {
		log.Errorf("complete config failed: %v", err)
		return err
	}
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, c config.PluginConfig) types.Action {
	skipCache, _ := proxywasm.GetHttpRequestHeader(SKIP_CACHE_HEADER)
	if skipCache == "on" {
		ctx.SetContext(SKIP_CACHE_HEADER, struct{}{})
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	// cache from request header
	if c.CacheKeyFromHeader != "" {
		key, _ := proxywasm.GetHttpRequestHeader(c.CacheKeyFromHeader)
		if key == "" {
			log.Warnf("[onHttpRequestHeaders] cache key from header: %s is empty, skip cache", c.CacheKeyFromHeader)
			ctx.DontReadRequestBody()
			return types.ActionContinue
		}
		log.Debugf("[onHttpRequestHeaders] cache key from request header: %s, key: %s", c.CacheKeyFromHeader, key)

		ctx.SetContext(CACHE_KEY_CONTEXT_KEY, key)

		if err := CheckCacheForKey(key, ctx, c); err != nil {
			log.Errorf("[onHttpRequestHeaders] check cache for key: %s failed, error: %v", key, err)
		}
		ctx.DisableReroute()
		_ = proxywasm.RemoveHttpRequestHeader("Accept-Encoding")
		ctx.DontReadRequestBody()
		return types.ActionPause
	}

	// cache from request body but does not have a body or not json format
	contentType, _ := proxywasm.GetHttpRequestHeader("content-type")

	if contentType == "" || !strings.Contains(contentType, "application/json") {
		log.Warnf("[onHttpRequestHeaders] content is not json, can't process: %s", contentType)
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	ctx.SetRequestBodyBufferLimit(DEFAULT_MAX_BODY_BYTES)

	ctx.DisableReroute()
	_ = proxywasm.RemoveHttpRequestHeader("Accept-Encoding")
	// The request has a body and requires delaying the header transmission until a cache miss occurs,
	// at which point the header should be sent.
	return types.HeaderStopIteration
}

func onHttpRequestBody(ctx wrapper.HttpContext, c config.PluginConfig, body []byte) types.Action {
	var key string
	if c.CacheKeyFromBody != "" {
		bodyJson := gjson.ParseBytes(body)
		log.Debugf("[onHttpRequestBody] cache key from requestBody: %s", c.CacheKeyFromBody)

		key = bodyJson.Get(c.CacheKeyFromBody).String()
		if key == "" {
			log.Debugf("[onHttpRequestBody] parse key from request body failed")
			ctx.DontReadResponseBody()
			return types.ActionContinue
		}
	} else {
		key = string(body)
		log.Debugf("[onHttpRequestBody] cache key from requestWholeBody.")
	}

	log.Debugf("[onHttpRequestBody] key: %s", key)
	ctx.SetContext(CACHE_KEY_CONTEXT_KEY, key)

	if err := CheckCacheForKey(key, ctx, c); err != nil {
		log.Errorf("[onHttpRequestBody] check cache for key: %s failed, error: %v", key, err)
		return types.ActionContinue
	}

	return types.ActionPause
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, c config.PluginConfig) types.Action {
	skipCache := ctx.GetContext(SKIP_CACHE_HEADER)
	if skipCache != nil {
		log.Debugf("[onHttpResponseHeader] detect skip cache header, skip cache")
		proxywasm.AddHttpResponseHeader("x-cache-status", "skip")
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	key := ctx.GetContext(CACHE_KEY_CONTEXT_KEY)
	if key == nil {
		log.Debugf("[onHttpResponseHeader] cache key is nil, skip cache")
		proxywasm.AddHttpResponseHeader("x-cache-status", "skip")
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	status, err := proxywasm.GetHttpResponseHeader(":status")
	if err != nil {
		log.Errorf("[onHttpResponseHeader] unable to load :status header from response: %v, skip cache", err)
		proxywasm.AddHttpResponseHeader("x-cache-status", "skip")
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	// check if status allowed
	found := false
	respCode, _ := strconv.Atoi(status)
	for _, element := range c.CacheResponseCode {
		if element == int32(respCode) {
			found = true
			break
		}
	}
	if !found {
		log.Infof("[onHttpResponseHeader] status not allow to cached: %s, skip cache", status)
		proxywasm.AddHttpResponseHeader("x-cache-status", "skip")
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	if ctx.GetContext(CACHE_KEY_CONTEXT_KEY) != nil {
		if c.CacheValueFromBodyType == "original" {
			// If CacheValueFromBodyType is original, use the original response content-type
			respType, err := proxywasm.GetHttpResponseHeader("content-type")
			if err != nil {
				log.Errorf("[onHttpResponseHeader] get content-type error: %s, skip cache", err)
				proxywasm.AddHttpResponseHeader("x-cache-status", "skip")
				ctx.DontReadResponseBody()
				return types.ActionContinue
			}
			ctx.SetContext(CACHE_VALUE_RESP_TYPE, respType)
		} else {
			ctx.SetContext(CACHE_VALUE_RESP_TYPE, c.CacheValueFromBodyType)
		}
		proxywasm.AddHttpResponseHeader("x-cache-status", "miss")

	}
	ctx.SetResponseBodyBufferLimit(DEFAULT_MAX_BODY_BYTES)
	return types.ActionContinue
}

func onHttpResponseBody(ctx wrapper.HttpContext, c config.PluginConfig, body []byte) types.Action {
	key := ctx.GetContext(CACHE_KEY_CONTEXT_KEY)
	if key == nil {
		// will never reached here, this condition has been considered in onHttpResponseHeaders
		log.Debugf("[onHttpResponseBody] cache key is nil, skip cache")
		return types.ActionContinue
	}

	var value string
	if c.CacheValueFromBody != "" {
		//parse GJSON
		respType := ctx.GetContext(CACHE_VALUE_RESP_TYPE)
		if strings.Contains(respType, "application/json") {
			//cache json parse response body
			bodyJson := gjson.ParseBytes(body)
			if !bodyJson.Exists() {
				log.Warnf("[onHttpResponseBody] parse json from non json response body: %s", body)
				return types.ActionContinue
			}
			value = bodyJson.Get(c.CacheValueFromBody).String()
			if strings.TrimSpace(value) == "" {
				log.Warnf("[onHttpResponseBody] parse value from response body failed, body:%s", body)
				return types.ActionContinue
			}
		}
		//If there are other body types, add a parsing process here.
	} else {
		value = string(body)
	}

	cacheResponse(ctx, c, key.(string), value)
	return types.ActionContinue

}
