# llama.cpp Server API Gaps

Assessment of this application against the API documented in the llama.cpp
server README.

## Scope and interpretation

The application in this repository is a Go client/helper library for a
llama.cpp-compatible server. It does not expose a llama.cpp-compatible HTTP
server of its own. Therefore, “supported” below means that the application has
an implemented client operation that can call the endpoint and consume its
response. A route is marked **Partial** when the application can call it or
retain its raw response, but does not model the endpoint's full documented
contract.

The comparison is against the `master` version of the official
[`tools/server/README.md`](https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md),
reviewed on 2026-09-08. The upstream document is mutable; this assessment
should be refreshed when llama.cpp is upgraded.

## Difficulty scale

The score estimates implementation effort in this application, not llama.cpp
runtime or model-support complexity.

| Score | Meaning |
| --- | --- |
| 1 | Small wrapper and response type; little new behavior |
| 2 | Straightforward HTTP operation plus validation/tests |
| 3 | New request/response models or streaming/format handling |
| 4 | Substantial protocol, model, or state handling |
| 5 | Cross-cutting feature, binary/cache state, or a new server subsystem |

## Endpoint coverage

### Core server endpoints

| Official endpoint | Application status | Evidence / gap | Difficulty if missing |
| --- | --- | --- | --- |
| `GET /health` | **Full** | Implemented by `CheckHealth`; also falls back to `/v1/health`. | — |
| `GET /v1/health` | **Full** | Implemented as the health-check fallback. | — |
| `POST /completion` | **Full** | Implemented by `Complete`, `StreamComplete`, and the completion branch of inference. | — |
| `POST /tokenize` | **Missing** | No tokenizer client method or response type. | **2/5** — add a small JSON request/response wrapper and tests. |
| `POST /detokenize` | **Missing** | No detokenizer client method or response type. | **2/5** — add a small JSON request/response wrapper and tests. |
| `POST /apply-template` | **Missing** | No chat-template formatting operation. | **2/5** — add request/response types and a client method. |
| `POST /embedding` | **Missing** | No non-OpenAI embedding operation. | **3/5** — add embedding request/response models, including multimodal and normalization options. |
| `POST /reranking` (`/rerank`, `/v1/rerank`, `/v1/reranking`) | **Missing** | No reranking request or result model. | **3/5** — add model/query/document types and result decoding; runtime requires a reranker model. |
| `POST /infill` | **Missing** | No code-infill operation or stream decoder specialized for infill. | **3/5** — reuse streaming infrastructure, but add FIM-specific request fields and tests. |
| `GET /props` | **Partial** | `fetchProps` calls the endpoint and preserves the raw JSON, but `PropsData` only models selected fields. | **1/5** — extend response models if callers need the complete documented schema. |
| `POST /props` | **Missing** | No operation for changing server properties; upstream requires the server's `--props` option. | **2/5** — add a JSON POST method once the upstream request schema is defined. |
| `POST /embeddings` | **Missing** | No non-OpenAI embeddings operation; this differs from `/v1/embeddings` and supports unpooled token embeddings. | **3/5** — add batch and nested embedding response types. |
| `GET /slots` | **Full** | `fetchSlots` calls the endpoint and decodes the slot array into `SlotsData`; used by telemetry and the live overlay. | — |
| `GET /metrics` | **Partial** | `fetchMetrics` preserves raw Prometheus text and parses simple metric/value pairs, but does not fully model labels or Prometheus semantics. | **1/5** — improve parsing only if labeled metrics or typed queries are needed. |
| `POST /slots/{id_slot}?action=save` | **Missing** | No prompt-cache save operation. | **5/5** — requires binary/cache-file lifecycle, path/configuration handling, and response modeling. |
| `POST /slots/{id_slot}?action=restore` | **Missing** | No prompt-cache restore operation. | **5/5** — same cache/state concerns as save, plus validation and error handling. |
| `POST /slots/{id_slot}?action=erase` | **Missing** | No prompt-cache erase operation. | **4/5** — simpler than save/restore, but still requires slot-state mutation and safety checks. |
| `GET /lora-adapters` | **Missing** | No LoRA adapter listing operation. | **2/5** — add a typed GET method and adapter models. |
| `POST /lora-adapters` | **Missing** | No global LoRA scale/update operation. | **2/5** — add a typed POST method and validation. |

### Router/model-management endpoints

These endpoints are available when llama.cpp is run in router mode (typically
without a model argument).

| Official endpoint | Application status | Evidence / gap | Difficulty if missing |
| --- | --- | --- | --- |
| `GET /models` | **Full** | `fetchModelRecords` tries `/models` first and normalizes runtime/model status records. | — |
| `POST /models/load` | **Full** | `LoadModel` posts the model name and waits for the resulting runtime state. | — |
| `POST /models/unload` | **Full** | `UnloadModel` posts the model name and waits for the resulting runtime state. | — |
| `GET /models/sse` | **Partial** | `monitorModelLoading` consumes SSE lifecycle events for a selected model, but does not expose a general-purpose model-event stream API. | **2/5** — expose the existing parser through a public callback/iterator. |
| `POST /models` | **Missing** | No non-blocking model-download operation. | **3/5** — add download request/ack types and coordinate with the existing SSE progress stream. |
| `DELETE /models` | **Missing** | No cached-model deletion operation. | **2/5** — add a safe query-parameter DELETE method and response handling. |

### OpenAI-compatible endpoints

| Official endpoint | Application status | Evidence / gap | Difficulty if missing |
| --- | --- | --- | --- |
| `GET /v1/models` | **Full** | `fetchModelRows` and `ListLoadedModels` call and normalize the endpoint. | — |
| `POST /v1/completions` | **Full** | Used by `infer`/streaming when a request has no messages; response and SSE decoding are implemented. | — |
| `POST /v1/chat/completions` | **Full** | Implemented by chat inference and streaming operations, including reasoning-content handling and tools in request models. | — |
| `POST /v1/chat/completions/control` | **Missing** | No operation for controlling an in-flight completion (`reasoning_end`). | **4/5** — requires concurrent request coordination and completion-ID lifecycle support. |
| `POST /v1/responses` | **Missing** | No Responses API request conversion or response-item model. | **4/5** — a substantial new protocol surface, even though llama.cpp converts it internally to chat completions. |
| `POST /v1/embeddings` | **Missing** | No OpenAI-compatible embeddings method or vector response model. | **3/5** — add string/batch inputs, encoding options, and response decoding. |
| `POST /v1/responses/input_tokens` | **Missing** | No input-token counting method. | **2/5** — add a request wrapper and `{object,input_tokens}` response. |
| `POST /v1/chat/completions/input_tokens` | **Missing** | No chat-completion token-counting method. The llama.cpp README documents this as a convenience endpoint, not an official OpenAI endpoint. | **2/5** — reuse chat request models and add a count response. |

### Anthropic-compatible endpoints

| Official endpoint | Application status | Evidence / gap | Difficulty if missing |
| --- | --- | --- | --- |
| `POST /v1/messages` | **Missing** | No Anthropic request/response types or SSE event handling. Existing chat types are OpenAI-shaped. | **4/5** — requires content-block mapping, system/tool fields, Anthropic response envelopes, and streaming events. |
| `POST /v1/messages/count_tokens` | **Missing** | This is the reported gap: no request is made to this route and no `{input_tokens}` response type exists. | **2/5** — add a method accepting the Messages payload and decode the token count; no generation or stream handling is needed. |

## Not counted as supported llama.cpp API

The application has `UploadFile` and `DeleteFile` helpers for `/v1/files`, but
`/v1/files` is not listed in the llama.cpp server README's API endpoint
sections. Those helpers should not be treated as evidence that the llama.cpp
server implements an official file API; they are optional compatibility helpers
and may return 404 against a standard llama.cpp server.

The README also documents `/tools` for the web UI. It explicitly says that this
is an internal endpoint subject to change and should not be used by downstream
applications, so it is excluded from the compatibility target here.

## Priority recommendations

1. Implement `POST /v1/messages/count_tokens` first: it directly addresses the
   reported compatibility gap and is a low-effort, stateless operation.
2. Add the equivalent `POST /v1/responses/input_tokens` and
   `/v1/chat/completions/input_tokens` methods if clients need provider-neutral
   token budgeting.
3. Add `/tokenize`, `/detokenize`, and `/apply-template` as shared primitives;
   they are small and useful for prompt inspection and preflight validation.
4. Add `/v1/embeddings`, `/rerank`, and `/v1/messages` only when the application
   needs those model capabilities; they introduce new result schemas and model
   assumptions.
5. Defer slot cache mutation, LoRA global mutation, and Responses API support
   until there is a concrete caller, because they carry more state or protocol
   surface than the current inference client requires.

## Summary

Counting endpoint families and documented aliases as separate callable routes,
the application has full support for health, completion, chat completion, model
listing, and slots, with partial support for properties and metrics. The main
missing endpoint relevant to the current request is `POST
/v1/messages/count_tokens`; its estimated implementation difficulty is **2/5**.
The broader gaps are concentrated in tokenization utilities, embeddings and
reranking, alternate provider protocols, mutable server state, and prompt-cache
management.
