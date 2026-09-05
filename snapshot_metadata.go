package induction

import "strings"

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
		return
	}
	snapshot.ApplicationToolsUsed = snapshotHasApplicationToolUse(messages)
	snapshot.ApplicationTools = snapshot.ApplicationToolsUsed
}

func messagesOrNil(req *ChatRequest) []Message {
	if req == nil {
		return nil
	}
	return req.Messages
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
