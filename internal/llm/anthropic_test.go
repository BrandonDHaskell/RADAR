package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func withTestEndpoint(t *testing.T, server *httptest.Server) {
	t.Helper()
	original := anthropicEndpoint
	anthropicEndpoint = server.URL
	t.Cleanup(func() { anthropicEndpoint = original })
}

func jsonMessageResponse(t *testing.T, verdict Verdict) []byte {
	t.Helper()
	text, err := json.Marshal(verdict)
	if err != nil {
		t.Fatalf("marshaling verdict fixture: %v", err)
	}
	resp := anthropicResponse{
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{{Type: "text", Text: string(text)}},
		StopReason: "end_turn",
	}
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshaling response fixture: %v", err)
	}
	return body
}

func TestAnthropicProviderFitVerdictSendsExpectedRequest(t *testing.T) {
	var gotAPIKey, gotVersion string
	var gotReq anthropicRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		w.Write(jsonMessageResponse(t, Verdict{
			Verdict:        "pursue",
			MatchedRoleTag: "automation-engineer",
			Reasoning:      "Strong match on automation experience.",
		}))
	}))
	defer server.Close()
	withTestEndpoint(t, server)

	provider := NewAnthropicProvider("test-key", "claude-haiku-4-5")
	verdict, err := provider.FitVerdict(context.Background(), "system prompt", "user prompt")
	if err != nil {
		t.Fatalf("FitVerdict: %v", err)
	}

	if gotAPIKey != "test-key" {
		t.Errorf("x-api-key header = %q, want %q", gotAPIKey, "test-key")
	}
	if gotVersion != anthropicVersion {
		t.Errorf("anthropic-version header = %q, want %q", gotVersion, anthropicVersion)
	}
	if gotReq.Model != "claude-haiku-4-5" {
		t.Errorf("request Model = %q, want %q", gotReq.Model, "claude-haiku-4-5")
	}
	if gotReq.System != "system prompt" {
		t.Errorf("request System = %q, want %q", gotReq.System, "system prompt")
	}
	if gotReq.OutputConfig.Format.Type != "json_schema" {
		t.Errorf("request OutputConfig.Format.Type = %q, want %q", gotReq.OutputConfig.Format.Type, "json_schema")
	}
	if verdict.Verdict != "pursue" || verdict.MatchedRoleTag != "automation-engineer" {
		t.Errorf("verdict = %+v", verdict)
	}
}

func TestAnthropicProviderFitVerdictHandlesRefusal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := anthropicResponse{StopReason: "refusal"}
		body, _ := json.Marshal(resp)
		w.Write(body)
	}))
	defer server.Close()
	withTestEndpoint(t, server)

	provider := NewAnthropicProvider("test-key", "claude-haiku-4-5")
	_, err := provider.FitVerdict(context.Background(), "system", "user")
	if err == nil {
		t.Fatal("FitVerdict: got nil error, want an error for a refusal")
	}
}

func TestAnthropicProviderFitVerdictStripsCodeFences(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := anthropicResponse{
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{{Type: "text", Text: "```json\n{\"verdict\":\"skip\",\"matched_role_tag\":\"automation-engineer\",\"reasoning\":\"No match.\"}\n```"}},
			StopReason: "end_turn",
		}
		body, _ := json.Marshal(resp)
		w.Write(body)
	}))
	defer server.Close()
	withTestEndpoint(t, server)

	provider := NewAnthropicProvider("test-key", "claude-haiku-4-5")
	verdict, err := provider.FitVerdict(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("FitVerdict: %v", err)
	}
	if verdict.Verdict != "skip" {
		t.Errorf("verdict.Verdict = %q, want %q", verdict.Verdict, "skip")
	}
}

func TestAnthropicProviderFitVerdictRejectsMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := anthropicResponse{
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{{Type: "text", Text: "not json at all"}},
			StopReason: "end_turn",
		}
		body, _ := json.Marshal(resp)
		w.Write(body)
	}))
	defer server.Close()
	withTestEndpoint(t, server)

	provider := NewAnthropicProvider("test-key", "claude-haiku-4-5")
	_, err := provider.FitVerdict(context.Background(), "system", "user")
	if err == nil {
		t.Fatal("FitVerdict: got nil error, want a parse error for malformed JSON")
	}
}

func TestAnthropicProviderFitVerdictRetriesOn500(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write(jsonMessageResponse(t, Verdict{
			Verdict:        "stretch",
			MatchedRoleTag: "technical-program-manager",
			Reasoning:      "Partial match.",
		}))
	}))
	defer server.Close()
	withTestEndpoint(t, server)

	provider := NewAnthropicProvider("test-key", "claude-haiku-4-5")
	_, err := provider.FitVerdict(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("FitVerdict: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}
