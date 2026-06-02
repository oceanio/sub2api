package apicompat

import "strings"

// Fork addition (CLAUDE.md §5): handling for Responses tool types the upstream
// convertResponsesToAnthropicTools switch doesn't recognise. Kept in a separate
// file so the upstream converter only carries a one-line hook.
//
// The bug this fixes: the upstream `default` branch passed every unknown tool
// type through verbatim, including type="namespace". Anthropic has no such tag,
// so the request 400s with:
//
//	tools.N: Input tag 'namespace' found using 'type' does not match any of the
//	expected tags: 'bash_20250124', ... 'web_search_20260209'
//
// Codex (Responses API) groups MCP / sub tools under a type="namespace" tool
// whose children live in `tools[]`. Anthropic only understands flat function
// tools, so we expand the namespace into qualified function tools. Other OpenAI
// built-ins that Anthropic doesn't support are dropped rather than passed
// through (they would 400 the same way).

// forkExpandResponsesTool converts a Responses tool whose type the upstream
// switch handed to its default branch. It returns zero or more Anthropic tools.
func forkExpandResponsesTool(t ResponsesTool) []AnthropicTool {
	if t.Type == "namespace" {
		return expandResponsesNamespaceTool(t)
	}
	if isUnsupportedOpenAIBuiltinToolType(t.Type) {
		return nil
	}
	// Preserve the previous behaviour for any other unknown type: pass it
	// through verbatim (best-effort; named custom tools may legitimately use a
	// type Anthropic accepts).
	return []AnthropicTool{{
		Type:        t.Type,
		Name:        t.Name,
		Description: t.Description,
		InputSchema: t.Parameters,
	}}
}

// expandResponsesNamespaceTool flattens a type="namespace" tool into one
// Anthropic function tool per child, with the child name qualified by the
// namespace so it stays unique and round-trips back to Codex unchanged.
func expandResponsesNamespaceTool(t ResponsesTool) []AnthropicTool {
	var out []AnthropicTool
	for _, child := range t.Tools {
		name := qualifyResponsesNamespaceToolName(t.Name, child.Name)
		if name == "" {
			continue
		}
		out = append(out, AnthropicTool{
			Name:        name,
			Description: child.Description,
			InputSchema: normalizeAnthropicInputSchema(child.Parameters),
		})
	}
	return out
}

// qualifyResponsesNamespaceToolName builds the "<namespace>__<child>" name Codex
// uses for namespaced tools. Names already prefixed (or MCP names) are left
// alone so the value round-trips unchanged. Mirrors CLIProxyAPI.
func qualifyResponsesNamespaceToolName(namespaceName, childName string) string {
	childName = strings.TrimSpace(childName)
	if childName == "" || namespaceName == "" || strings.HasPrefix(childName, "mcp__") {
		return childName
	}
	if strings.HasPrefix(childName, namespaceName) {
		return childName
	}
	if strings.HasSuffix(namespaceName, "__") {
		return namespaceName + childName
	}
	return namespaceName + "__" + childName
}

// isUnsupportedOpenAIBuiltinToolType reports whether the type is an OpenAI
// built-in tool that has no Anthropic equivalent and must be dropped.
func isUnsupportedOpenAIBuiltinToolType(toolType string) bool {
	switch toolType {
	case "image_generation", "file_search", "code_interpreter", "computer_use_preview":
		return true
	default:
		return false
	}
}
