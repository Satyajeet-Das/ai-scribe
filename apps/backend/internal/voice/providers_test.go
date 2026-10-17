package voice

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
	"github.com/Satyajeet-Das/ai-scribe/internal/voice/intent"
)

func groqVoiceConfig(baseURL string) config.VoiceConfig {
	v := config.VoiceConfig{Enabled: true}
	v.Intent.Provider = config.VoiceProviderGroq
	v.Intent.APIKey = "gsk_test"
	v.Intent.BaseURL = baseURL
	v.ApplyDefaults()
	return v
}

func TestBuildProviders_GroqIsConsultedOnlyAfterTheExactCommandRules(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, "Bearer gsk_test", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"intent\":\"repeat_question\",\"option_key\":\"\",\"confidence\":0.9}"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	p, err := BuildProviders(groqVoiceConfig(srv.URL))
	require.NoError(t, err)
	assert.Equal(t, "rules+groq:v1", p.Classifier.Name())

	in := intent.Input{QuestionType: "ESSAY"}

	in.Transcript = "next question"
	it, err := p.Classifier.Classify(context.Background(), in)
	require.NoError(t, err)
	assert.Equal(t, intent.NextQuestion, it.Name)
	assert.Zero(t, calls, "an exact command is understood locally and never sent to Groq")

	in.Transcript = "could you say the question once more please"
	it, err = p.Classifier.Classify(context.Background(), in)
	require.NoError(t, err)
	assert.Equal(t, intent.RepeatQuestion, it.Name)
	assert.Equal(t, 1, calls)
}

func TestBuildProviders_GroqReportsAnUnknownPromptVersion(t *testing.T) {
	cfg := groqVoiceConfig("http://127.0.0.1:1")
	cfg.Intent.PromptVersion = "no-such-version"
	_, err := BuildProviders(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no-such-version")
}

func TestBuildProviders_OtherIntentProvidersStillBuild(t *testing.T) {
	for _, provider := range []string{config.VoiceProviderRules, config.VoiceProviderOllama} {
		cfg := config.VoiceConfig{Enabled: true}
		cfg.Intent.Provider = provider
		cfg.ApplyDefaults()
		p, err := BuildProviders(cfg)
		require.NoError(t, err, provider)
		assert.NotNil(t, p.Classifier, provider)
	}
	cfg := config.VoiceConfig{Enabled: true}
	cfg.Intent.Provider = "mystery"
	cfg.ApplyDefaults()
	_, err := BuildProviders(cfg)
	assert.Error(t, err)
}
