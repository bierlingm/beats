package impetus

import "testing"

func TestInfer(t *testing.T) {
	tests := []struct {
		content string
		want    string
	}{
		{"github.com/foo/bar", "GitHub discovery"},
		{"https://github.com/user/repo", "GitHub discovery"},
		{"https://twitter.com/user/status/123", "X discovery"},
		{"https://x.com/user", "X discovery"},
		{"https://youtube.com/watch?v=abc", "YouTube discovery"},
		{"https://youtu.be/abc", "YouTube discovery"},
		{"From coaching: great insight", "Coaching"},
		{"coaching: notes from today", "Coaching"},
		{"session: weekly sync", "Session"},
		{"bug: fix null pointer", "Bug fix"},
		{"fix: broken link", "Bug fix"},
		{"feature: new dashboard", "Feature"},
		{"implemented: user auth", "Feature"},
		{"https://linkedin.com/in/user", "LinkedIn discovery"},
		{"https://reddit.com/r/golang", "Reddit discovery"},
		{"https://example.com/page", "Web discovery"},
		{"plain text without url", ""},
	}

	for _, tt := range tests {
		got := Infer(tt.content)
		if got != tt.want {
			t.Errorf("Infer(%q) = %q, want %q", tt.content, got, tt.want)
		}
	}
}

func TestInferWithConfidence(t *testing.T) {
	label, conf := InferWithConfidence("github.com/foo")
	if label != "GitHub discovery" || conf != 1.0 {
		t.Errorf("got (%q, %f), want (GitHub discovery, 1.0)", label, conf)
	}

	label, conf = InferWithConfidence("https://example.com")
	if label != "Web discovery" || conf != 0.5 {
		t.Errorf("got (%q, %f), want (Web discovery, 0.5)", label, conf)
	}

	label, conf = InferWithConfidence("plain text")
	if label != "" || conf != 0.0 {
		t.Errorf("got (%q, %f), want ('', 0.0)", label, conf)
	}
}
