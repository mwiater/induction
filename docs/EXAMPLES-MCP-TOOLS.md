# MCP Tool Pipeline Examples

Examples:

- [RSS aggregation](#mcp-tool-discovery-and-rss-aggregation)
- [Weather lookup](#chained-mcp-geocoding-and-weather-lookup)
- [Website research](#mcp-website-research-with-source-verification)
- [JSON inspection](#mcp-json-endpoint-inspection)
- [Web comparison](#mcp-multi-source-web-comparison)

## MCP tool discovery and RSS aggregation

### 1. Overview

Demonstrates MCP discovery followed by retrieval from two RSS sources and source-grouped consolidation. It is useful for checking available tools and building feed-ingestion workflows.

### 2. Steps

- **discover:** Lists available tools and their details.
- **fetchRSS1:** Gathers items from the Willamette Week RSS feed.
- **fetchRSS2:** Gathers items from the KOIN Portland RSS feed.
- **list:** Groups all retrieved feed items by source.

### 3. Expected final output

All retrieved items grouped under the correct RSS source, with source identity preserved and no invented feed content.

### 4. Example YAML

[Open pipelines/pipeline.mcp-01.yaml in the repository](../pipelines/pipeline.mcp-01.yaml)

### 5. YAML source

```yaml
name: MCP Test

steps:
  - name: discover
    model: Agents-A1-MTP-Apex-I-Quality
    systemPrompt: You are a helpful assistant.
    userPrompt: List the tools and tool details you have available to you.

  - name: fetchRSS1
    model: Agents-A1-MTP-Apex-I-Quality
    systemPrompt: You are a helpful assistant.
    userPrompt: "Gather the items in this RSS feed: https://www.wweek.com/arc/outboundfeeds/rss/?outputType=xml"

  - name: fetchRSS2
    model: Agents-A1-MTP-Apex-I-Quality
    systemPrompt: You are a helpful assistant.
    userPrompt: "Gather the items in this RSS feed: https://www.koin.com/news/portland/feed/"

  - name: list
    model: Agents-A1-MTP-Apex-I-Quality
    systemPrompt: You are a helpful assistant.
    userPrompt: List all of the feed items you found from each source, grouped by source.
```

## Chained MCP geocoding and weather lookup

### 1. Overview

Demonstrates dependent tool calls where geocoding supplies coordinates to a weather lookup. It is useful for travel planning and for testing multi-tool context handoff.

### 2. Steps

- **locate-city:** Geocodes Portland, Oregon, uses the returned coordinates with weather, and reports only returned facts.
- **prepare-weather-brief:** Creates a practical three-day brief tied to available temperature, precipitation, or wind data.

### 3. Expected final output

Coordinates, available three-day forecast facts, and a clothing or scheduling consideration for each reported condition; missing values remain unknown.

### 4. Example YAML

[Open pipelines/pipeline.mcp-02.yaml in the repository](../pipelines/pipeline.mcp-02.yaml)

### 5. YAML source

```yaml
name: MCP Weather Brief

steps:
  - name: locate-city
    model: LFM-2.5-8B-A1B-UD-Q8_K_XL
    systemPrompt: |
      You are a travel-planning assistant. Use the configured MCP tools when
      live location data is needed, and distinguish tool results from advice.
    userPrompt: |
      Use the `geocode` tool to locate Portland, Oregon. Then use the returned
      coordinates with the `weather` tool to retrieve the forecast. Report the
      coordinates and forecast facts clearly; do not guess missing values.

  - name: prepare-weather-brief
    model: LFM-2.5-8B-A1B-UD-Q8_K_XL
    systemPrompt: You turn the preceding live weather lookup into a practical, concise travel brief. Do not invent conditions that were not returned by the tools.
    userPrompt: Create a three-day planning brief from the preceding tool results. Include temperatures, precipitation or wind information when available, and one clothing or scheduling consideration tied to each reported condition.
```

## MCP website research with source verification

### 1. Overview

Combines bounded site search with a verification fetch. It is useful for source-grounded research where claims need URLs and an authoritative page should be checked before writing.

### 2. Steps

- **research-site:** Searches Project Gutenberg, visits no more than five relevant pages, and records claims with URLs.
- **verify-and-explain:** Fetches the most authoritative result as text, verifies wording, and writes the research note.

### 3. Expected final output

A general-reader explanation, three URL-supported takeaways, and a limitations note, based only on retrieved material.

### 4. Example YAML

[Open pipelines/pipeline.mcp-03.yaml in the repository](../pipelines/pipeline.mcp-03.yaml)

### 5. YAML source

```yaml
name: MCP Website Research

steps:
  - name: research-site
    model: GLM-4.7-Flash-Q4_K_M
    systemPrompt: Use read-only website tools for focused research. Keep citations or URLs attached to the facts they support.
    userPrompt: |
      Use `search_site` starting at https://www.gutenberg.org/ to find public
      information about how Project Gutenberg makes ebooks available. Visit no
      more than five relevant pages and collect the key claims with their URLs.

  - name: verify-and-explain
    model: GLM-4.7-Flash-Q4_K_M
    systemPrompt: You write a source-grounded research note from the preceding browser results. Do not add facts that are absent from those results.
    userPrompt: Summarize the site's ebook-access model for a general reader. Use `fetch_url_as_text` on the most authoritative page found in the preceding research to verify its wording, then provide three sourced takeaways and a short limitations note.
```

## MCP JSON endpoint inspection

### 1. Overview

Discovers direct JSON endpoints for a GitHub repository and then makes one precise read-only request. It is useful for endpoint selection and structured API investigation.

### 2. Steps

- **discover-api-data:** Uses discover_json to identify direct JSON responses or embedded JSON and likely fields.
- **fetch-api-record:** Uses fetch_url_as_JSON on the best candidate and separates returned fields from interpretation.

### 3. Expected final output

Repository name, description, default branch, issue count, and last-updated field when present, with the selected URL retained and absent fields stated explicitly.

### 4. Example YAML

[Open pipelines/pipeline.mcp-04.yaml in the repository](../pipelines/pipeline.mcp-04.yaml)

### 5. YAML source

```yaml
name: MCP JSON Endpoint Inspection

steps:
  - name: discover-api-data
    model: Muse-Glimmer-30B-Q4_K_XL
    systemPrompt: You are an API investigation assistant. Use only read-only MCP tools and explain what each response establishes.
    userPrompt: |
      Use `discover_json` on https://api.github.com/repos/openai/openai-python
      to identify direct JSON responses or embedded JSON data. Report the most
      useful endpoint candidates and what fields they appear to contain.

  - name: fetch-api-record
    model: Muse-Glimmer-30B-Q4_K_XL
    systemPrompt: Use the preceding discovery results to make one precise read-only JSON request. Preserve the URL and distinguish returned fields from interpretation.
    userPrompt: Use `fetch_url_as_JSON` on the best direct JSON endpoint found in the preceding step. Summarize the repository's name, description, default branch, issue count, and last-updated field when present. State explicitly when a field is absent.
```

## MCP multi-source web comparison

### 1. Overview

Fetches current text from RFC Editor and IANA and compares the sites for developers. It is useful for source selection and technical-information architecture research.

### 2. Steps

- **collect-pages:** Fetches both pages as text and records each site's purpose, information types, and prominent links separately.
- **compare-sources:** Produces a side-by-side comparison and recommends a starting site for each developer need with URLs.

### 3. Expected final output

A side-by-side comparison, site purposes, published information types, recommendations for standards/protocol/registry needs, and supporting URLs, while preserving uncertainty from incomplete pages.

### 4. Example YAML

[Open pipelines/pipeline.mcp-05.yaml in the repository](../pipelines/pipeline.mcp-05.yaml)

### 5. YAML source

```yaml
name: MCP Web Content Comparison

steps:
  - name: collect-pages
    model: Qwen-3-Coder-Next-Q4_K_M
    systemPrompt: Use read-only MCP tools to retrieve public web content. Keep the two sources distinct and avoid unsupported conclusions.
    userPrompt: |
      Use `fetch_url_as_text` to retrieve the current content of
      https://www.rfc-editor.org/ and https://www.iana.org/. Identify each
      site's stated purpose, the kinds of information it publishes, and any
      prominent navigation or resource links.

  - name: compare-sources
    model: Qwen-3-Coder-Next-Q4_K_M
    systemPrompt: You are a technical-information analyst. Base the comparison only on the preceding fetched page text and preserve uncertainty caused by incomplete pages.
    userPrompt: Compare the two sites for a developer who needs standards, protocol references, or registry information. Give a side-by-side summary, recommend which site to start with for each need, and cite the relevant URL for each recommendation.
```
