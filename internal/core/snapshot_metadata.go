// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"path/filepath"
	"strings"
)

const (
	// snapshotInputText identifies a fixed value used internally by this package.
	snapshotInputText = "text"
	// snapshotInputVision identifies a fixed value used internally by this package.
	snapshotInputVision = "vision"
	// snapshotInputAudio identifies a fixed value used internally by this package.
	snapshotInputAudio = "audio"
	// snapshotInputDocument identifies a fixed value used internally by this package.
	snapshotInputDocument = "document"
	// snapshotOutputText identifies a fixed value used internally by this package.
	snapshotOutputText = "text"
	// snapshotOutputJSON identifies a fixed value used internally by this package.
	snapshotOutputJSON = "json"
	// snapshotOutputJSONSchema identifies a fixed value used internally by this package.
	snapshotOutputJSONSchema = "json_schema"
	// snapshotOutputGrammar identifies a fixed value used internally by this package.
	snapshotOutputGrammar = "grammar"
	// snapshotOutputClassify identifies a fixed value used internally by this package.
	snapshotOutputClassify = "classification"
)

// snapshotInputType performs the internal operation described by its signature.
func snapshotInputType(req *ChatRequest) string {
	if req == nil {
		return snapshotInputText
	}
	if strings.TrimSpace(req.DocumentFilename) != "" {
		return snapshotInputDocument
	}
	if len(req.ImageData) > 0 {
		return snapshotInputVision
	}
	hasAudio, hasDocument, hasVision := false, false, false
	for _, message := range req.Messages {
		parts, ok := message.Content.([]ContentPart)
		if !ok {
			continue
		}
		for _, part := range parts {
			if strings.HasPrefix(strings.TrimSpace(part.Text), "Document filename: ") {
				hasDocument = true
			}
			switch strings.ToLower(part.Type) {
			case "audio", "audio_url", "input_audio":
				hasAudio = true
			case "file":
				hasDocument = true
			case "vision", "image", "image_url":
				hasVision = true
			}
		}
	}
	if hasDocument {
		return snapshotInputDocument
	}
	if hasAudio {
		return snapshotInputAudio
	}
	if hasVision {
		return snapshotInputVision
	}
	return snapshotInputText
}

// snapshotOutputType performs the internal operation described by its signature.
func snapshotOutputType(req *ChatRequest) string {
	if req == nil {
		return snapshotOutputText
	}
	if req.Classification != nil {
		return snapshotOutputClassify
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

// snapshotHasApplicationToolUse performs the internal operation described by its signature.
func snapshotHasApplicationToolUse(messages []Message) bool {
	for _, message := range messages {
		if strings.EqualFold(message.Role, "tool") || (strings.EqualFold(message.Role, "assistant") && len(message.ToolCalls) > 0) {
			return true
		}
	}
	return false
}

// initializeSnapshotMetadata performs the internal operation described by its signature.
func initializeSnapshotMetadata(snapshot *ModelSnapshot, req *ChatRequest) {
	initializeSnapshotMetadataForMCP(snapshot, req, false)
}

// initializeSnapshotMetadataForMCP performs the internal operation described by its signature.
func initializeSnapshotMetadataForMCP(snapshot *ModelSnapshot, req *ChatRequest, mcpTools bool) {
	initializeSnapshotMetadataForMCPWithNames(snapshot, req, mcpTools, nil)
}

// initializeSnapshotMetadataForMCPWithNames performs the internal operation described by its signature.
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

// mcpToolArguments performs the internal operation described by its signature.
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

// messagesOrNil performs the internal operation described by its signature.
func messagesOrNil(req *ChatRequest) []Message {
	if req == nil {
		return nil
	}
	return req.Messages
}

// snapshotMessages performs the internal operation described by its signature.
func snapshotMessages(req *ChatRequest) []Message {
	if req == nil {
		return nil
	}
	messages := cloneMessages(req.Messages)
	return redactMessages(messages, req.ImageFilename, req.DocumentFilename)
}

// redactMessages performs the internal operation described by its signature.
// Additional context: cloneMessages intentionally keeps content values shallow. Copy the multimodal parts and nested image metadata before redacting them so the live inference request retains its original image data URL.
func redactMessages(messages []Message, imageFilename, documentFilename string) []Message {
	for i := range messages {
		parts, ok := messages[i].Content.([]ContentPart)
		if !ok {
			continue
		}

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
			clonedObject := make(map[string]any, len(object))
			for key, item := range object {
				clonedObject[key] = item
			}
			if kind, _ := clonedObject["type"].(string); strings.EqualFold(kind, "image_url") {
				if imageURL, ok := clonedObject["image_url"].(map[string]any); ok {
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
					clonedObject["image_url"] = imageCopy
				}
			}
			if kind, _ := clonedObject["type"].(string); strings.EqualFold(kind, "text") {
				if value, ok := clonedObject["text"].(string); ok && strings.HasPrefix(value, "Document filename: ") {
					filename := documentFilename
					if filename == "" {
						filename = strings.TrimSpace(strings.TrimPrefix(strings.SplitN(value, "\n", 2)[0], "Document filename: "))
					}
					clonedObject["text"] = filepath.Base(filename)
				}
			}
			parts[j] = clonedObject
		}
		messages[i].Content = parts
	}
	return messages
}

// toolUseOutcome performs the internal operation described by its signature.
func toolUseOutcome(available, used bool) string {
	if !available {
		return "not_available"
	}
	if used {
		return "used_successfully"
	}
	return "model_did_not_request"
}
