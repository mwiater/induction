package induction

import (
	"context"
	"testing"
	"time"
)

func TestApplicationToolLoopCarriesLoadTimeToFinalSnapshot(t *testing.T) {
	turn := 0
	firstSnapshot := &ModelSnapshot{ModelLoadTime: 3 * time.Second}
	finalSnapshot := &ModelSnapshot{}
	responseWithCall := &InferenceResponse{Choices: []InferenceChoice{{Message: &InferenceResponseMessage{ToolCalls: []InferenceToolCall{{ID: "call-1", Function: InferenceFunctionCall{Name: "local", Arguments: "{}"}}}}}}}
	responseFinal := &InferenceResponse{Choices: []InferenceChoice{{Message: &InferenceResponseMessage{Content: "done"}}}}
	response, snapshot, err := runApplicationToolLoopWith(context.Background(), &ChatRequest{Model: "model"}, func(context.Context, string, string) (string, error) { return "result", nil }, nil, func(context.Context, *ChatRequest) (*InferenceResponse, *ModelSnapshot, error) {
		turn++
		if turn == 1 {
			return responseWithCall, firstSnapshot, nil
		}
		return responseFinal, finalSnapshot, nil
	})
	if err != nil || response != responseFinal || snapshot != finalSnapshot {
		t.Fatalf("loop result response=%#v snapshot=%#v err=%v", response, snapshot, err)
	}
	if finalSnapshot.ModelLoadTime != 3*time.Second {
		t.Fatalf("final snapshot load time = %v, want 3s", finalSnapshot.ModelLoadTime)
	}
}
