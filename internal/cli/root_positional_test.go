package cli

import "testing"

func TestClassifyRootInvocation(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		flags     []string
		delimiter bool
		wantRoute rootInvocationRoute
		wantError bool
	}{
		{name: "model selection", wantRoute: rootModelSelection},
		{name: "normal with explicit flag", flags: []string{"config"}, wantRoute: rootNormal},
		{name: "prompt", args: []string{"Hello!"}, wantRoute: rootPositionalChat},
		{name: "preserves content", args: []string{"  tabs\tand --model inside  "}, wantRoute: rootPositionalChat},
		{name: "unicode", args: []string{"こんにちは 👋"}, wantRoute: rootPositionalChat},
		{name: "multiple", args: []string{"Hello", "World"}, wantError: true},
		{name: "empty", args: []string{""}, wantError: true},
		{name: "whitespace", args: []string{" \t\n "}, wantError: true},
		{name: "explicit flag", args: []string{"Hello!"}, flags: []string{"model"}, wantError: true},
		{name: "delimiter", args: []string{"-hello"}, delimiter: true, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := classifyRootInvocation(tt.args, tt.flags, tt.delimiter)
			if (err != nil) != tt.wantError {
				t.Fatalf("error = %v, wantError=%v", err, tt.wantError)
			}
			if err == nil && got != tt.wantRoute {
				t.Fatalf("route = %v, want %v", got, tt.wantRoute)
			}
		})
	}
}

func TestClassifyRootInvocationDoesNotMutatePrompt(t *testing.T) {
	prompt := "  keep leading and trailing whitespace  "
	route, err := classifyRootInvocation([]string{prompt}, nil, false)
	if err != nil || route != rootPositionalChat {
		t.Fatalf("classification = %v, %v", route, err)
	}
	if prompt != "  keep leading and trailing whitespace  " {
		t.Fatal("classification mutated the prompt")
	}
}
