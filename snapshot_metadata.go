package induction

import (
	"path/filepath"
	"strings"
)

const (
	snapshotInputText        = "text"
	snapshotInputImage       = "image"
	snapshotInputVision      = "vision"
	snapshotOutputText       = "text"
	snapshotOutputJSON       = "json"
	snapshotOutputJSONSchema = "json_schema"
	snapshotOutputGrammar    = "grammar"
)

func snapshotInputType(req *ChatRequest) string {
	if req == nil {
		return snapshotInputText
	}
	if len(req.ImageData) > 0 {
		return snapshotInputImage
	}
	for _, message := range req.Messages {
		parts, ok := message.Content.([]ContentPart)
		if !ok {
			continue
		}
		for _, part := range parts {
			switch strings.ToLower(part.Type) {
			case "vision":
				return snapshotInputVision
			case "image", "image_url":
				return snapshotInputImage
			}
		}
	}
	return snapshotInputText
}

func snapshotOutputType(req *ChatRequest) string {
	if req == nil {
		return snapshotOutputText
	}
	if strings.TrimSpace(req.Grammar) != "" {
		return snapshotOutputGrammar
	}
	if req.JSONSchema != nil {
		return snapshotOutputJSONSchema
	}
	if req.ResponseFormat == nil {
		return snapshotOutputText
	}
	switch strings.ToLower(strings.TrimSpace(req.ResponseFormat.Type)) {
	case "json_object", "json":
		return snapshotOutputJSON
	case "json_schema":
		return snapshotOutputJSONSchema
	case "grammar":
		return snapshotOutputGrammar
	default:
		return snapshotOutputText
	}
}

func snapshotHasApplicationToolUse(messages []Message) bool {
	for _, message := range messages {
		if strings.EqualFold(message.Role, "tool") || (strings.EqualFold(message.Role, "assistant") && len(message.ToolCalls) > 0) {
			return true
		}
	}
	return false
}

func initializeSnapshotMetadata(snapshot *ModelSnapshot, req *ChatRequest) {
	initializeSnapshotMetadataForMCP(snapshot, req, false)
}

func initializeSnapshotMetadataForMCP(snapshot *ModelSnapshot, req *ChatRequest, mcpTools bool) {
	initializeSnapshotMetadataForMCPWithNames(snapshot, req, mcpTools, nil)
}

func initializeSnapshotMetadataForMCPWithNames(snapshot *ModelSnapshot, req *ChatRequest, mcpTools bool, mcpToolNames []string) {
	snapshot.InputType = snapshotInputType(req)
	snapshot.OutputType = snapshotOutputType(req)
	messages := messagesOrNil(req)
	snapshot.ApplicationToolsAvailable = !mcpTools && req != nil && len(req.Tools) > 0
	snapshot.MCPToolsAvailable = mcpTools
	snapshot.MCPToolNames = append([]string(nil), mcpToolNames...)
	snapshot.ApplicationToolUseOutcome = toolUseOutcome(snapshot.ApplicationToolsAvailable, snapshotHasApplicationToolUse(messages))
	snapshot.MCPToolUseOutcome = toolUseOutcome(snapshot.MCPToolsAvailable, snapshotHasApplicationToolUse(messages))
	if mcpTools {
		snapshot.MCPToolsUsed = snapshotHasApplicationToolUse(messages)
		snapshot.MCPTools = snapshot.MCPToolsUsed
		snapshot.MCPToolsUsedArguments = mcpToolArguments(messages)
		return
	}
	snapshot.ApplicationToolsUsed = snapshotHasApplicationToolUse(messages)
	snapshot.ApplicationTools = snapshot.ApplicationToolsUsed
}

func mcpToolArguments(messages []Message) map[string][]string {
	var used map[string][]string
	for _, message := range messages {
		if !strings.EqualFold(message.Role, "assistant") {
			continue
		}
		for _, call := range message.ToolCalls {
			if used == nil {
				used = make(map[string][]string)
			}
			used[call.Function.Name] = append(used[call.Function.Name], call.Function.Arguments)
		}
	}
	return used
}

func messagesOrNil(req *ChatRequest) []Message {
	if req == nil {
		return nil
	}
	return req.Messages
}

func snapshotMessages(req *ChatRequest) []Message {
	if req == nil {
		return nil
	}
	messages := cloneMessages(req.Messages)
	return redactMessages(messages, req.ImageFilename, req.DocumentFilename)
}

func redactMessages(messages []Message, imageFilename, documentFilename string) []Message {
	for i := range messages {
		parts, ok := messages[i].Content.([]ContentPart)
		if !ok {
			continue
		}
		// cloneMessages intentionally keeps content values shallow. Copy the
		// multimodal parts and nested image metadata before redacting them so
		// the live inference request retains its original image data URL.
		parts = append([]ContentPart(nil), parts...)
		for j := range parts {
			if parts[j].ImageURL != nil {
				imageURL := *parts[j].ImageURL
				parts[j].ImageURL = &imageURL
			}
		}
		for j := range parts {
			switch strings.ToLower(parts[j].Type) {
			case "image", "image_url":
				if parts[j].ImageURL != nil && (strings.HasPrefix(strings.ToLower(parts[j].ImageURL.URL), "data:") || parts[j].ImageURL.URL == "image") {
					filename := imageFilename
					if filename == "" {
						filename = "image"
					}
					parts[j].ImageURL.URL = filepath.Base(filename)
				}
			case "text":
				if strings.HasPrefix(parts[j].Text, "Document filename: ") {
					filename := documentFilename
					if filename == "" {
						filename = strings.TrimSpace(strings.TrimPrefix(strings.SplitN(parts[j].Text, "\n", 2)[0], "Document filename: "))
					}
					parts[j].Text = filepath.Base(filename)
				}
			}
		}
		messages[i].Content = parts
		continue
	}
	for i := range messages {
		parts, ok := messages[i].Content.([]any)
		if !ok {
			continue
		}
		for j, value := range parts {
			object, ok := value.(map[string]any)
			if !ok {
				continue
			}
			copy := make(map[string]any, len(object))
			for key, item := range object {
				copy[key] = item
			}
			if kind, _ := copy["type"].(string); strings.EqualFold(kind, "image_url") {
				if imageURL, ok := copy["image_url"].(map[string]any); ok {
					imageCopy := make(map[string]any, len(imageURL))
					for key, item := range imageURL {
						imageCopy[key] = item
					}
					if url, _ := imageCopy["url"].(string); strings.HasPrefix(strings.ToLower(url), "data:") || url == "image" {
						filename := imageFilename
						if filename == "" {
							filename = "image"
						}
						imageCopy["url"] = filepath.Base(filename)
					}
					copy["image_url"] = imageCopy
				}
			}
			if kind, _ := copy["type"].(string); strings.EqualFold(kind, "text") {
				if value, ok := copy["text"].(string); ok && strings.HasPrefix(value, "Document filename: ") {
					filename := documentFilename
					if filename == "" {
						filename = strings.TrimSpace(strings.TrimPrefix(strings.SplitN(value, "\n", 2)[0], "Document filename: "))
					}
					copy["text"] = filepath.Base(filename)
				}
			}
			parts[j] = copy
		}
		messages[i].Content = parts
	}
	return messages
}

func toolUseOutcome(available, used bool) string {
	if !available {
		return "not_available"
	}
	if used {
		return "used_successfully"
	}
	return "model_did_not_request"
}
