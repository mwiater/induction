package induction

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderSessionTranscript writes a persisted chat session using the same
// labels, icons, colors, and reasoning presentation as the console UI.
func RenderSessionTranscript(out io.Writer, session *ChatSession) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}
	first := true
	for _, snapshot := range session.Snapshots {
		if snapshot == nil {
			continue
		}
		user := lastSessionMessage(snapshot.Messages, "user")
		assistant := lastSessionMessage(snapshot.Messages, "assistant")
		content := replayAssistantContent(snapshot, assistant)
		if user == nil && content == "" {
			continue
		}
		if !first {
			if _, err := fmt.Fprintln(out); err != nil {
				return err
			}
		}
		first = false
		if user != nil {
			if _, err := fmt.Fprint(out, renderReplayText(defaultConsoleUITheme.mainUserPrompt, " "+renderWhiteIcon(DefaultUnicode.Star), " You: ", replayMessageContent(user.Content))); err != nil {
				return err
			}
		}
		if content != "" {
			if user != nil {
				if _, err := fmt.Fprintln(out); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprint(out, renderReplayAssistantMessage(content)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}
	if first {
		for _, message := range session.Messages {
			if message.Role == "system" {
				continue
			}
			content := replayMessageContent(message.Content)
			style := defaultConsoleUITheme.mainAssistantContent
			if message.Role == "user" {
				style = defaultConsoleUITheme.mainUserPrompt
			}
			if _, err := fmt.Fprint(out, renderReplayText(style, " "+renderWhiteIcon(replayIcon(message.Role)), " "+replayLabel(message.Role)+": ", content)); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderReplayText(style lipgloss.Style, icon, label, content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		prefix := " "
		if i == 0 {
			prefix = icon + label
		}
		lines[i] = style.Render(prefix + line)
	}
	return strings.Join(lines, "\n")
}

func renderReplayAssistantMessage(content string) string {
	var out strings.Builder
	iconPrefix := " " + renderWhiteIcon(DefaultUnicode.Sparkle)
	label := " Assistant: "
	for len(content) > 0 {
		start := strings.Index(content, "<think>")
		if start < 0 {
			out.WriteString(renderReplayText(defaultConsoleUITheme.mainAssistantContent, iconPrefix, label, content))
			break
		}
		if start > 0 {
			out.WriteString(renderReplayText(defaultConsoleUITheme.mainAssistantContent, iconPrefix, label, content[:start]))
			iconPrefix = ""
			label = ""
		}
		content = content[start:]
		end := strings.Index(content, "</think>")
		if end < 0 {
			out.WriteString(renderReplayText(defaultConsoleUITheme.mainAssistantReasoning, iconPrefix, label, content))
			break
		}
		end += len("</think>")
		out.WriteString(renderReplayText(defaultConsoleUITheme.mainAssistantReasoning, iconPrefix, label, content[:end]))
		iconPrefix = ""
		label = ""
		content = content[end:]
	}
	return out.String()
}

func lastSessionMessage(messages []Message, role string) *Message {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == role {
			return &messages[i]
		}
	}
	return nil
}

func replayAssistantContent(snapshot *ModelSnapshot, assistant *Message) string {
	content := ""
	if assistant != nil {
		content = replayMessageContent(assistant.Content)
	}
	if len(snapshot.Interaction) == 0 {
		return content
	}
	interaction := snapshot.Interaction[len(snapshot.Interaction)-1]
	if interaction.Content == "" && interaction.ReasoningContent == "" {
		return content
	}
	if interaction.ReasoningContent != "" {
		return "<think>" + interaction.ReasoningContent + "</think>" + interaction.Content
	}
	return interaction.Content
}

func replayMessageContent(content any) string {
	if value, ok := content.(string); ok {
		return value
	}
	if parts, ok := content.([]ContentPart); ok {
		var text strings.Builder
		for _, part := range parts {
			text.WriteString(part.Text)
		}
		if text.Len() > 0 {
			return text.String()
		}
	}
	if parts, ok := content.([]any); ok {
		var text strings.Builder
		for _, part := range parts {
			if object, ok := part.(map[string]any); ok {
				if value, ok := object["text"].(string); ok {
					text.WriteString(value)
				}
			}
		}
		if text.Len() > 0 {
			return text.String()
		}
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return fmt.Sprint(content)
	}
	return string(encoded)
}

func replayIcon(role string) string {
	if role == "user" {
		return DefaultUnicode.Star
	}
	return DefaultUnicode.Sparkle
}

func replayLabel(role string) string {
	switch role {
	case "user":
		return "You"
	case "assistant":
		return "Assistant"
	case "tool":
		return "Tool"
	default:
		return role
	}
}
