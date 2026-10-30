package extract

import "context"

type disabledModel struct{}

// Disabled is the model used while no extraction provider is configured. Uploads
// are refused before any job exists, so it is never asked to read a document; if
// it is, it fails with a permanent error and sends nothing anywhere.
func Disabled() Model { return disabledModel{} }

func (disabledModel) Name() string          { return "disabled" }
func (disabledModel) PromptVersion() string { return "" }
func (disabledModel) CanReadImages() bool   { return false }
func (disabledModel) Complete(context.Context, Request) (string, error) {
	return "", NewError(CodeRejected, false, "no extraction provider is configured")
}
