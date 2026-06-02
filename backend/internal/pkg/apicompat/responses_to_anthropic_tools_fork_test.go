package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A type="namespace" tool must be flattened into qualified Anthropic function
// tools; the "namespace" type must never reach Anthropic (it 400s otherwise).
func TestConvertResponsesToAnthropicTools_NamespaceFlattened(t *testing.T) {
	tools := []ResponsesTool{
		{Type: "function", Name: "plain", Description: "p", Parameters: json.RawMessage(`{"type":"object"}`)},
		{
			Type: "namespace",
			Name: "github",
			Tools: []ResponsesTool{
				{Type: "function", Name: "list_repos", Description: "ls", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)},
				{Type: "function", Name: "create_issue", Parameters: nil},
			},
		},
	}

	out := convertResponsesToAnthropicTools(tools)

	require.Len(t, out, 3)
	for _, tl := range out {
		assert.NotEqual(t, "namespace", tl.Type, "namespace type must not survive conversion")
	}
	assert.Equal(t, "plain", out[0].Name)
	assert.Equal(t, "github__list_repos", out[1].Name)
	assert.Equal(t, "ls", out[1].Description)
	assert.Equal(t, "github__create_issue", out[2].Name)
	// Missing parameters are normalised to a valid empty object schema.
	assert.JSONEq(t, `{"type":"object","properties":{}}`, string(out[2].InputSchema))
}

// MCP-prefixed and already-qualified child names are passed through unchanged.
func TestConvertResponsesToAnthropicTools_NamespaceNamePreserved(t *testing.T) {
	tools := []ResponsesTool{{
		Type: "namespace",
		Name: "srv",
		Tools: []ResponsesTool{
			{Type: "function", Name: "mcp__already_qualified"},
			{Type: "function", Name: "srv__alsoqualified"},
			{Type: "function", Name: ""}, // dropped: no name
		},
	}}

	out := convertResponsesToAnthropicTools(tools)

	require.Len(t, out, 2)
	assert.Equal(t, "mcp__already_qualified", out[0].Name)
	assert.Equal(t, "srv__alsoqualified", out[1].Name)
}

// Unsupported OpenAI built-in tool types have no Anthropic equivalent and must
// be dropped rather than passed through (pass-through 400s).
func TestConvertResponsesToAnthropicTools_UnsupportedBuiltinDropped(t *testing.T) {
	tools := []ResponsesTool{
		{Type: "image_generation"},
		{Type: "file_search"},
		{Type: "code_interpreter"},
		{Type: "computer_use_preview"},
		{Type: "function", Name: "keep", Parameters: json.RawMessage(`{"type":"object"}`)},
	}

	out := convertResponsesToAnthropicTools(tools)

	require.Len(t, out, 1)
	assert.Equal(t, "keep", out[0].Name)
}

// Unknown-but-named tool types retain the previous best-effort pass-through.
func TestConvertResponsesToAnthropicTools_UnknownTypePassedThrough(t *testing.T) {
	tools := []ResponsesTool{{Type: "custom", Name: "x", Parameters: json.RawMessage(`{"type":"object"}`)}}

	out := convertResponsesToAnthropicTools(tools)

	require.Len(t, out, 1)
	assert.Equal(t, "custom", out[0].Type)
	assert.Equal(t, "x", out[0].Name)
}
