package server

import "strings"

// parseTagList reads [{keyField,valField},...] tag arrays from JSON bodies.
func parseTagList(v any, keyField, valField string, trimKey bool) (map[string]string, error) {
	raw, ok := v.([]any)
	if !ok {
		return nil, nil
	}
	out := make(map[string]string, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		k, _ := m[keyField].(string)
		val, _ := m[valField].(string)
		if trimKey {
			k = strings.TrimSpace(k)
		}
		if k == "" {
			continue
		}
		out[k] = val
	}
	return out, nil
}

// parseKeyValueTags parses Events/SSM/Secrets Tags arrays ([{Key,Value},...]).
func parseKeyValueTags(v any) map[string]string {
	raw, ok := v.([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out, err := parseTagList(v, "Key", "Value", true)
	if err != nil || len(out) == 0 {
		return nil
	}
	return out
}

func parseKMSTags(v any) map[string]string {
	raw, ok := v.([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out, err := parseTagList(v, "TagKey", "TagValue", true)
	if err != nil || len(out) == 0 {
		return nil
	}
	return out
}

func parseEMRTags(raw any) map[string]string {
	if _, ok := raw.([]any); !ok {
		return nil
	}
	out, _ := parseTagList(raw, "Key", "Value", false)
	return out
}

// parseResourceGroupTagMap parses Resource Groups Tagging API Tags objects.
func parseResourceGroupTagMap(v any) map[string]string {
	tagsRaw, _ := v.(map[string]any)
	tags := map[string]string{}
	for k, val := range tagsRaw {
		tags[k] = anyToString(val)
	}
	return tags
}
