---
title: Response Cache
keywords: [higress,response cache]
description: Response Cache Plugin Configuration Reference
---
## Function Description
Response caching plugin supports extracting keys from request headers/request bodies and caching values extracted from response bodies. On subsequent requests, if the request headers/request bodies contain the same key, it directly returns the cached value without forwarding the request to the backend service.

**Hint**

When carrying the request header `x-higress-skip-response-cache: on`, the current request will not use content from the cache but will be directly forwarded to the backend service. Additionally, the response content from this request will not be cached.

## Runtime Properties
Plugin Execution Phase: `Authentication Phase`
Plugin Execution Priority: `10`

## Configuration Description

### Cache Service (cache)
| Property | Type | Requirement | Default | Description |
| --- | --- | --- | --- | --- |
| cache.type | string | required | "" | Cache service type, e.g., redis |
| cache.serviceName | string | required | "" | Cache service name |
| cache.serviceHost | string | required | "" | Cache service domain |
| cache.servicePort | int64 | optional | 6379 | Cache service port |
| cache.username | string | optional | "" | Cache service username |
| cache.password | string | optional | "" | Cache service password |
| cache.timeout | uint32 | optional | 10000 | Timeout for cache service in milliseconds. Default is 10000, i.e., 10 seconds |
| cache.cacheTTL | int | optional | 0 | Cache expiration time in seconds. Default is 0, meaning never expires |
| cacheKeyPrefix | string | optional | "higress-response-cache:" | Prefix for cache keys, default is "higress-response-cache:" |                 |

### Other Configurations
| Name | Type | Requirement | Default | Description |
| --- | --- | --- | --- | --- |
| cacheResponseCode | array of number | optional | 200 | Indicates the list of response status codes that support caching; the default is 200.|
| cacheKeyFromHeader | string | required | "" | Indicates extracting the value of a fixed field from header as cache key; this field does not take effect when configured as empty; `cacheKeyFromHeader` and `cacheKeyFromBody` **only one of them can be configured as non-empty** when both are non-empty |
| cacheKeyFromBody | string | required | "" | Indicates extracting a string as cache key from request Body based on [GJSON PATH](https://github.com/tidwall/gjson/blob/master/SYNTAX.md) syntax in `application/json` response format; when this field is configured as empty, it means extracting the entire body as cache key |
| cacheValueFromBodyType | string | optional | "application/json" | Indicates the type of cached body, content-type will return this value when cache is hit; default is `application/json`; when configured as special value `original`, it means using the response type as part of the cached content, and return the original response type when cache is hit |
| cacheValueFromBody | string | optional | "" | Indicates that when the response `Content-Type` is `application/json`, it supports extracting a string as cache value from response Body based on [GJSON PATH](https://github.com/tidwall/gjson/blob/master/SYNTAX.md) syntax; when this field is configured as empty, it means extracting the entire body as cache value |

### Cache Configuration Description
The cache key concatenation logic is as follows:

[**Note:** `cacheKeyFromHeader` and `cacheKeyFromBody` only support one non-empty configuration, and `cacheKeyFromHeader` has higher priority than `cacheKeyFromBody` in non-empty cases]

1. `cacheKeyFromHeader != ""`, then the cache key is: `cacheKeyPrefix` + the content extracted from the `cacheKeyFromHeader` corresponding field in the request header. If the corresponding field does not exist in the request header, this request will skip following process of cache plugin.
2. `cacheKeyFromHeader = ""` and `cacheKeyFromBody = ""`, then the cache key is: `cacheKeyPrefix` + request body. If the request body is empty, this request will skip following process of cache plugin.
3. `cacheKeyFromHeader = ""` and `cacheKeyFromBody != ""`, then the cache key is: `cacheKeyPrefix` + the content extracted from the `cacheKeyFromBody` corresponding field in the request body. If the corresponding field does not exist in the request body extraction, this request will skip following process of cache plugin.


When processed by cache plugin, the response header uses `x-cache-status` to indicate three states:
- `x-cache-status: hit`, indicates that the cache was hit and the cached content is returned directly
- `x-cache-status: miss`, indicates that the cache was not hit and the backend response result is returned
- `x-cache-status: skip`, indicates that the cache check was skipped and the backend response result is returned; including cases where the extracted value is incorrect occurred in onHttpRequestHeaders, onHttpRequestBody and onHttpResponseHeaders
 
When hitted the cache, the type of response is determined by `cacheValueFromBodyType`:
- When `cacheValueFromBodyType = "orignal"`, the `Content-Type` returned in the response is the `Content-Type` of the original request's corresponding response before caching.
- When `cacheValueFromBodyType != "orignal"`, the `Content-Type` returned in the response is the result configured by `cacheValueFromBodyType`; the current default configuration is `application/json`.

  
## Configuration Example
### Basic Configuration
```yaml
cache:
  type: redis
  serviceName: my-redis.dns
  servicePort: 6379
  timeout: 2000
  
cacheKeyFromHeader: "x-http-cache-key"

cacheValueFromBodyType: "application/json"
cacheValueFromBody: "messages.@reverse.0.content"
```

Assumed Request

```bash
# Request
curl -H "x-http-cache-key: abcd" <url>

# Response
{"messages":[{"content":"1"}, {"content":"2"}, {"content":"3"}]}
```

In this case, the cache key would be `higress-response-cache:abcd`, and the cached value would be `3`.

For subsequent requests that hit the cache, the response Content-Type returned is `application/json`.

### Response Body as Cache Value
To cache all response bodies, configure as follows:

```yaml
cacheValueFromBodyType: "text/html"
cacheValueFromBody: ""
```
For subsequent requests that hit the cache, the response Content-Type returned is `text/html`.


### Request Body as Cache Key
To use the request body as the key, configure as follows:

```yaml

cacheKeyFromBody: ""
```

The configuration supports GJSON PATH syntax.


## Advanced Usage
When the body is `application/json`, GJSON PATH syntax is supported:

For example, the expression `messages.@reverse.0.content` means taking the content of the first item after reversing the messages array.

GJSON PATH also supports conditional syntax. For instance, to take the content of the last message where role is "user", you can write: `messages.@reverse.#(role=="user").content`.

To concatenate all contents where role is "user" into an array, you can write: `messages.@reverse.#(role=="user")#.content`.

Pipeline syntax is also supported. For example, to take the second content where role is "user", you can write: `messages.@reverse.#(role=="user")#.content|1`.

Refer to the [official documentation](https://github.com/tidwall/gjson/blob/master/SYNTAX.md) for more usage examples, and test the syntax using the [GJSON Playground](https://gjson.dev/).

## Common Issues
1. If the error `error status returned by host: bad argument occurs`, check whether `serviceName` correctly includes the service type suffix (.dns, etc.).
2. If the error `gRPC config for type.googleapis.com/envoy.config.core.v3.TypedExtensionConfig rejected: Unable to create Wasm HTTP filter`, check if `servicePort` is configured correctly; for example, `.static` service type needs to configure the port as `80`.