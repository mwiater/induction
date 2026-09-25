package eval

import "testing"

func TestBaseURL(t *testing.T) {
	for input, want := range map[string]string{
		"http://localhost:9998":    "http://localhost:9998/v1",
		"http://localhost:9998/":   "http://localhost:9998/v1",
		"http://localhost:9998/v1": "http://localhost:9998/v1",
	} {
		got, err := BaseURL(input)
		if err != nil || got != want {
			t.Errorf("%q -> %q, %v", input, got, err)
		}
	}
}
