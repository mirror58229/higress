# Introduction
---
title: Generic Response Cache
keywords: [higress,response cache]
description: Generic response cache plugin configuration reference
---

**Note**

> The data plane's proxy wasm version needs to be greater than or equal to 0.2.100
> When compiling, you need to include the version tag, for example: `tinygo build -o main.wasm -scheduler=none -target=wasi -gc=custom -tags="custommalloc nottinygc_finalizer proxy_wasm_version_0_2_100" ./`
>

## Function Description

Generic response cache plugin supports extracting keys from request headers/request body, extracting values from response body and caching them; when the next request comes, if the request header/request body carries the same key, it will directly return the value in the cache instead of requesting the backend service.

**Tip**

When carrying the request header `x-higress-skip-response-cache: on`, the current request will not use the content in the cache, but will be directly forwarded to the backend service, and the response content of the request will not be cached either.


## Runtime Properties

Plugin execution stage: `Authentication Stage`
Plugin execution priority: `10`

## Configuration Description
Configuration includes cache database (cache) configuration section, and cache content configuration section

## Cache Service (cache)
| cache.type | string | required | "" | Cache service type, for example redis |
| --- | --- | --- | --- | --- |
| cache.serviceName | string | required | "" | Cache service name |
| cache.serviceHost | string | required | "" | Cache service domain |
| cache.servicePort | int64 | optional | 6379 | Cache service port |
| cache.username | string | optional | ""  | Cache service username |
| cache.password | string | optional | "" | Cache service password |
| cache.timeout | uint32 | optional | 10000 | Cache service timeout in milliseconds. The default value is 10000, which is 10 seconds |
| cache.cacheTTL | int | optional | 0 | Cache expiration time in seconds. The default value is 0, which means never expires |
| cacheKeyPrefix | string | optional | "higress-response-cache:" | Cache Key prefix, the default value is "higress-response-cache:" |


## Other Configurations
| Name | Type | Requirement | Default | Description |
| --- | --- | --- | --- | --- |
| cacheResponseCode | array of number | optional | 200 | Indicates the list of response status codes that support caching; the default is 200 |
| cacheKeyFromHeader | string | required | "" | Indicates extracting the value of a fixed field in the header as the cache key; this field does not take effect when configured as empty; `cacheKeyFromHeader` and `cacheKeyFromBody` **only one of them can be configured as non-empty** when non-empty, and they cannot be configured as non-empty at the same time |
| cacheKeyFromBody | string | required | "" | Indicates that according to the `application/json` response format, extract the string from the request Body based on [GJSON PATH](https://github.com/tidwall/gjson/blob/master/SYNTAX.md) syntax as the cache key; when this field is configured as empty, it means extracting all body as the cache key |
| cacheValueFromBodyType | string | optional | "application/json" | Indicates the type of cached body, when hitting cache, content-type will return this value; the default is `application/json`; when configured as empty, it indicates using the response type as part of the cached content |
| cacheValueFromBody | string | optional | "" | Indicates that when the response `Content-Type` is `application/json`, it supports extracting strings from the response Body based on [GJSON PATH](https://github.com/tidwall/gjson/blob/master/SYNTAX.md) syntax as the cache value; when this field is configured as empty, it means extracting all body as the cache value |

### Cache Configuration Description
The cache key concatenation logic is as follows:

[**Note:** `cacheKeyFromHeader` and `cacheKeyFromBody` only support one non-empty configuration, and `cacheKeyFromHeader` has higher priority than `cacheKeyFromBody` in non-empty cases]

1. `cacheKeyFromHeader != ""`, then the cache key is: `cacheKeyPrefix` + the content extracted from the `cacheKeyFromHeader` corresponding field in the request header; if the corresponding field does not exist in the request header, the cache will be skipped for this request
2. `cacheKeyFromHeader = ""` and `cacheKeyFromBody = ""`, then the cache key is: `cacheKeyPrefix` + request body; if the request body is empty, the cache will be skipped for this request
3. `cacheKeyFromHeader = ""` and `cacheKeyFromBody != ""`, then the cache key is: `cacheKeyPrefix` + the content extracted from the `cacheKeyFromBody` corresponding field in the request body; if the corresponding field does not exist in the request body extraction, the cache will be skipped for this request


When the cache plugin is hit, the response header uses `x-cache-status` to indicate three states:
- `x-cache-status: hit`, indicates that the cache was hit and the cached content is returned directly
- `x-cache-status: miss`, indicates that the cache was not hit and the backend response result is returned
- `x-cache-status: skip`, indicates that the cache check was skipped and the backend response result is returned; including all cases where the extracted value is incorrect
 
When the cache is hit, the response type is determined by `cacheValueFromBodyType`:
- When `cacheValueFromBodyType != ""`, the `Content-Type` returned in the response is the result configured by `cacheValueFromBodyType`; the current default configuration is `application/json`.
- When `cacheValueFromBodyType = ""`, the `Content-Type` returned in the response is the `Content-Type` of the original request's corresponding response before caching.

## Configuration Examples
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

Assuming the request is

```bash
# Request
curl -H "x-http-cache-key: abcd" <url>

# Response
{"messages":[{"content":"1"}, {"content":"2"}, {"content":"3"}]}
```

The cached key is `higress-response-cache:abcd`, and the cached value is `3`.

When subsequent requests hit the cache, the response Content-type returns `application/json`.


### Response body as value

If all response bodies are cached, you can configure as

```yaml
cacheValueFromBodyType: "text/html"
cacheValueFromBody: ""

```

When subsequent requests hit the cache, the response Content-type returns `text/html`.

### Request body as key

To use the request body as the key, you can configure as

```yaml
cacheKeyFromBody: ""
```

Configuration supports GJSON PATH syntax.

## Advanced Usage
When Body is `application/json`, it supports GJSON PATH syntax:

For example, the expression: `messages.@reverse.0.content`, which means reversing the messages array and taking the content of the first item;

GJSON PATH also supports conditional judgment syntax, for example, if you want to take the content of the last role as user as the key, you can write: `messages.@reverse.#(role=="user").content`;

If you want to concatenate all content with role as user into an array as the key, you can write: `messages.@reverse.#(role=="user")#.content`;

Pipeline syntax is also supported, for example, if you want to take the second-to-last content with role as user as the key, you can write: `messages.@reverse.#(role=="user")#.content|1`.

For more usage, please refer to the [official documentation](https://github.com/tidwall/gjson/blob/master/SYNTAX.md), and you can use [GJSON Playground](https://gjson.dev/) for syntax testing.

## FAQ

1. If the returned error is `error status returned by host: bad argument`, please check whether `serviceName` correctly includes the service type suffix (.dns, etc.).
2. If the returned error is `gRPC config for type.googleapis.com/envoy.config.core.v3.TypedExtensionConfig rejected: Unable to create Wasm HTTP filter`, please check whether `servicePort` is configured correctly; for example, `.static` type needs to configure the port as `80`.