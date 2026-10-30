package extract

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed prompts/*.md
var promptFS embed.FS

// DefaultPromptVersion is the prompt used unless another is chosen.
const DefaultPromptVersion = "v1"

// Prompt returns the embedded system prompt of a version.
func Prompt(version string) (string, error) {
	data, err := promptFS.ReadFile("prompts/" + version + ".md")
	if err != nil {
		return "", fmt.Errorf("unknown extraction prompt version %q", version)
	}
	return strings.TrimSpace(string(data)), nil
}
