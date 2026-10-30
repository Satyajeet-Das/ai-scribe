package extract

// Schema returns the JSON schema the model is asked to follow. It is written to
// satisfy strict structured output: every property is required and objects are
// closed, with optional values as unions with null. It is only a request, not a
// guarantee, so the reply is validated by parse whatever model produced it.
func Schema() map[string]any {
	str := map[string]any{"type": "string"}
	nullStr := map[string]any{"type": []string{"string", "null"}}
	nullInt := map[string]any{"type": []string{"integer", "null"}}
	integer := map[string]any{"type": "integer"}
	boolean := map[string]any{"type": "boolean"}

	obj := func(props map[string]any) map[string]any {
		req := make([]string, 0, len(props))
		for k := range props {
			req = append(req, k)
		}
		sortStrings(req)
		return map[string]any{"type": "object", "properties": props, "required": req, "additionalProperties": false}
	}
	arr := func(item any) map[string]any { return map[string]any{"type": "array", "items": item} }

	question := obj(map[string]any{
		"numberLabel":       str,
		"type":              map[string]any{"type": "string", "enum": []string{TypeMCQ, TypeEssay}},
		"instructions":      nullStr,
		"text":              str,
		"marks":             nullInt,
		"options":           arr(obj(map[string]any{"label": str, "text": str})),
		"tables":            arr(obj(map[string]any{"page": integer, "markdown": str})),
		"diagrams":          arr(obj(map[string]any{"page": integer, "region": nullStr, "description": nullStr})),
		"pages":             arr(integer),
		"continuesPrevious": boolean,
		"uncertain":         boolean,
	})
	return obj(map[string]any{
		"documentInstructions": nullStr,
		"questions":            arr(question),
		"unassigned":           arr(obj(map[string]any{"page": integer, "text": str, "reason": str})),
	})
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
