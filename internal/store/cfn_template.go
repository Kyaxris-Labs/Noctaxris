package store

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var cfnSubToken = regexp.MustCompile(`\$\{([^}]+)\}`)

// parseCFNTemplate accepts JSON or YAML TemplateBody and returns a typed template.
// YAML short-form tags (!Ref, !GetAtt, !Sub, !Join) are normalized to long form.
func parseCFNTemplate(templateBody string) (cfnTemplate, error) {
	templateBody = strings.TrimSpace(templateBody)
	if templateBody == "" {
		return cfnTemplate{}, fmt.Errorf("%w: TemplateBody is required", ErrCFNBadTemplate)
	}
	var tpl cfnTemplate
	if err := json.Unmarshal([]byte(templateBody), &tpl); err == nil {
		if len(tpl.Resources) == 0 {
			return cfnTemplate{}, fmt.Errorf("%w: Resources required", ErrCFNBadTemplate)
		}
		return tpl, nil
	}
	raw, err := parseCFNYAML(templateBody)
	if err != nil {
		return cfnTemplate{}, fmt.Errorf("%w: TemplateBody must be JSON or YAML", ErrCFNBadTemplate)
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return cfnTemplate{}, fmt.Errorf("%w: TemplateBody must be JSON or YAML", ErrCFNBadTemplate)
	}
	if err := json.Unmarshal(encoded, &tpl); err != nil {
		return cfnTemplate{}, fmt.Errorf("%w: TemplateBody must be JSON or YAML", ErrCFNBadTemplate)
	}
	if len(tpl.Resources) == 0 {
		return cfnTemplate{}, fmt.Errorf("%w: Resources required", ErrCFNBadTemplate)
	}
	return tpl, nil
}

func parseCFNYAML(body string) (map[string]any, error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(body), &root); err != nil {
		return nil, err
	}
	v, err := cfnYAMLNodeToAny(&root)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("template root must be a mapping")
	}
	return m, nil
}

func cfnYAMLNodeToAny(n *yaml.Node) (any, error) {
	if n == nil {
		return nil, nil
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return cfnYAMLNodeToAny(n.Content[0])
	case yaml.MappingNode:
		if intrinsic := cfnYAMLTaggedIntrinsic(n); intrinsic != nil {
			return intrinsic, nil
		}
		out := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			keyNode := n.Content[i]
			valNode := n.Content[i+1]
			key := keyNode.Value
			val, err := cfnYAMLNodeToAny(valNode)
			if err != nil {
				return nil, err
			}
			out[key] = val
		}
		return out, nil
	case yaml.SequenceNode:
		if intrinsic := cfnYAMLTaggedIntrinsic(n); intrinsic != nil {
			return intrinsic, nil
		}
		out := make([]any, 0, len(n.Content))
		for _, child := range n.Content {
			v, err := cfnYAMLNodeToAny(child)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case yaml.ScalarNode:
		if intrinsic := cfnYAMLTaggedIntrinsic(n); intrinsic != nil {
			return intrinsic, nil
		}
		return cfnYAMLDecodeScalar(n)
	case yaml.AliasNode:
		return cfnYAMLNodeToAny(n.Alias)
	default:
		return nil, fmt.Errorf("unsupported YAML node kind %v", n.Kind)
	}
}

func cfnYAMLTaggedIntrinsic(n *yaml.Node) any {
	// Short-form CFN tags use local tags like !Ref (not !!str / tag:yaml.org).
	if n.Tag == "" || strings.HasPrefix(n.Tag, "!!") || strings.HasPrefix(n.Tag, "tag:yaml.org") {
		return nil
	}
	if !strings.HasPrefix(n.Tag, "!") {
		return nil
	}
	tag := strings.TrimPrefix(n.Tag, "!")
	switch tag {
	case "Ref":
		if n.Kind == yaml.ScalarNode {
			return map[string]any{"Ref": n.Value}
		}
	case "Sub":
		v, err := cfnYAMLUntaggedValue(n)
		if err != nil {
			return nil
		}
		return map[string]any{"Fn::Sub": v}
	case "GetAtt":
		v, err := cfnYAMLUntaggedValue(n)
		if err != nil {
			return nil
		}
		switch t := v.(type) {
		case string:
			parts := strings.SplitN(t, ".", 2)
			if len(parts) == 2 {
				return map[string]any{"Fn::GetAtt": []any{parts[0], parts[1]}}
			}
			return map[string]any{"Fn::GetAtt": t}
		case []any:
			return map[string]any{"Fn::GetAtt": t}
		}
	case "Join":
		v, err := cfnYAMLUntaggedValue(n)
		if err != nil {
			return nil
		}
		return map[string]any{"Fn::Join": v}
	}
	return nil
}

func cfnYAMLUntaggedValue(n *yaml.Node) (any, error) {
	clone := *n
	clone.Tag = ""
	return cfnYAMLNodeToAny(&clone)
}

func cfnYAMLDecodeScalar(n *yaml.Node) (any, error) {
	switch n.Tag {
	case "!!null", "":
		if n.Value == "" && n.Tag == "!!null" {
			return nil, nil
		}
	}
	var v any
	if err := n.Decode(&v); err != nil {
		return n.Value, nil
	}
	return v, nil
}

type cfnEvalCtx struct {
	accountID   string
	region      string
	stackName   string
	refValues   map[string]string
	attributes  map[string]map[string]string
	resourceOrd []string
}

func newCFNEvalCtx(accountID, region, stackName string) *cfnEvalCtx {
	return &cfnEvalCtx{
		accountID:  accountID,
		region:     region,
		stackName:  stackName,
		refValues:  map[string]string{},
		attributes: map[string]map[string]string{},
	}
}

func (ctx *cfnEvalCtx) setResource(logicalID, refValue string, attrs map[string]string) {
	ctx.refValues[logicalID] = refValue
	ctx.attributes[logicalID] = attrs
	ctx.resourceOrd = append(ctx.resourceOrd, logicalID)
}

func (ctx *cfnEvalCtx) resolveProps(props map[string]any) (map[string]any, error) {
	if props == nil {
		return map[string]any{}, nil
	}
	out := make(map[string]any, len(props))
	for k, v := range props {
		rv, err := ctx.resolve(v)
		if err != nil {
			return nil, err
		}
		if rv == cfnNoValue {
			continue
		}
		out[k] = rv
	}
	return out, nil
}

var cfnNoValue = &struct{}{}

func (ctx *cfnEvalCtx) resolve(v any) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		if ref, ok := x["Ref"]; ok {
			return ctx.resolveRef(fmt.Sprint(ref))
		}
		if ga, ok := x["Fn::GetAtt"]; ok {
			return ctx.resolveGetAtt(ga)
		}
		if sub, ok := x["Fn::Sub"]; ok {
			return ctx.resolveSub(sub)
		}
		if join, ok := x["Fn::Join"]; ok {
			return ctx.resolveJoin(join)
		}
		out := make(map[string]any, len(x))
		for k, vv := range x {
			rv, err := ctx.resolve(vv)
			if err != nil {
				return nil, err
			}
			if rv == cfnNoValue {
				continue
			}
			out[k] = rv
		}
		return out, nil
	case []any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			rv, err := ctx.resolve(item)
			if err != nil {
				return nil, err
			}
			if rv == cfnNoValue {
				continue
			}
			out = append(out, rv)
		}
		return out, nil
	default:
		return v, nil
	}
}

func (ctx *cfnEvalCtx) resolveRef(name string) (any, error) {
	name = strings.TrimSpace(name)
	switch name {
	case "AWS::AccountId":
		return ctx.accountID, nil
	case "AWS::Region":
		return ctx.region, nil
	case "AWS::StackName":
		return ctx.stackName, nil
	case "AWS::NoValue":
		return cfnNoValue, nil
	}
	if v, ok := ctx.refValues[name]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("%w: unresolved Ref %q", ErrCFNBadTemplate, name)
}

func (ctx *cfnEvalCtx) resolveGetAtt(v any) (any, error) {
	logicalID, attr, err := parseGetAtt(v)
	if err != nil {
		return nil, err
	}
	attrs, ok := ctx.attributes[logicalID]
	if !ok {
		return nil, fmt.Errorf("%w: unresolved Fn::GetAtt %s.%s", ErrCFNBadTemplate, logicalID, attr)
	}
	val, ok := attrs[attr]
	if !ok {
		return nil, fmt.Errorf("%w: unsupported attribute %s.%s", ErrCFNBadTemplate, logicalID, attr)
	}
	return val, nil
}

func parseGetAtt(v any) (logicalID, attr string, err error) {
	switch t := v.(type) {
	case string:
		parts := strings.SplitN(t, ".", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("%w: Fn::GetAtt must be LogicalId.Attr", ErrCFNBadTemplate)
		}
		return parts[0], parts[1], nil
	case []any:
		if len(t) != 2 {
			return "", "", fmt.Errorf("%w: Fn::GetAtt list must have two elements", ErrCFNBadTemplate)
		}
		return fmt.Sprint(t[0]), fmt.Sprint(t[1]), nil
	default:
		return "", "", fmt.Errorf("%w: invalid Fn::GetAtt", ErrCFNBadTemplate)
	}
}

func (ctx *cfnEvalCtx) resolveSub(v any) (any, error) {
	var template string
	vars := map[string]string{}
	switch t := v.(type) {
	case string:
		template = t
	case []any:
		if len(t) == 0 {
			return "", fmt.Errorf("%w: empty Fn::Sub", ErrCFNBadTemplate)
		}
		template, _ = t[0].(string)
		if len(t) > 1 {
			m, ok := t[1].(map[string]any)
			if !ok {
				return "", fmt.Errorf("%w: Fn::Sub vars must be a map", ErrCFNBadTemplate)
			}
			for k, vv := range m {
				rv, err := ctx.resolve(vv)
				if err != nil {
					return nil, err
				}
				vars[k] = fmt.Sprint(rv)
			}
		}
	default:
		return "", fmt.Errorf("%w: invalid Fn::Sub", ErrCFNBadTemplate)
	}
	out := cfnSubToken.ReplaceAllStringFunc(template, func(match string) string {
		name := match[2 : len(match)-1]
		if v, ok := vars[name]; ok {
			return v
		}
		if strings.Contains(name, ".") {
			parts := strings.SplitN(name, ".", 2)
			if attrs, ok := ctx.attributes[parts[0]]; ok {
				if val, ok := attrs[parts[1]]; ok {
					return val
				}
			}
		}
		if v, ok := ctx.refValues[name]; ok {
			return v
		}
		switch name {
		case "AWS::AccountId":
			return ctx.accountID
		case "AWS::Region":
			return ctx.region
		case "AWS::StackName":
			return ctx.stackName
		}
		return match
	})
	if strings.Contains(out, "${") {
		return "", fmt.Errorf("%w: unresolved Fn::Sub in %q", ErrCFNBadTemplate, template)
	}
	return out, nil
}

func (ctx *cfnEvalCtx) resolveJoin(v any) (any, error) {
	arr, ok := v.([]any)
	if !ok || len(arr) != 2 {
		return "", fmt.Errorf("%w: Fn::Join requires [delimiter, list]", ErrCFNBadTemplate)
	}
	delim := fmt.Sprint(arr[0])
	list, ok := arr[1].([]any)
	if !ok {
		return "", fmt.Errorf("%w: Fn::Join list required", ErrCFNBadTemplate)
	}
	parts := make([]string, 0, len(list))
	for _, item := range list {
		rv, err := ctx.resolve(item)
		if err != nil {
			return nil, err
		}
		parts = append(parts, fmt.Sprint(rv))
	}
	return strings.Join(parts, delim), nil
}

func cfnDependencyOrder(resources map[string]cfnResource) ([]string, error) {
	deps := map[string]map[string]struct{}{}
	for id, res := range resources {
		deps[id] = map[string]struct{}{}
		collectCFNDeps(res.Properties, id, resources, deps[id])
	}
	var order []string
	ready := map[string]struct{}{}
	for id, d := range deps {
		if len(d) == 0 {
			ready[id] = struct{}{}
		}
	}
	for len(order) < len(resources) {
		if len(ready) == 0 {
			return nil, fmt.Errorf("%w: circular or unresolved resource dependencies", ErrCFNBadTemplate)
		}
		// Stable-ish pick: lowest name among ready.
		var next string
		for id := range ready {
			if next == "" || id < next {
				next = id
			}
		}
		delete(ready, next)
		order = append(order, next)
		for id, d := range deps {
			if _, done := indexOfString(order, id); done {
				continue
			}
			delete(d, next)
			if len(d) == 0 {
				ready[id] = struct{}{}
			}
		}
	}
	return order, nil
}

func indexOfString(list []string, want string) (int, bool) {
	for i, v := range list {
		if v == want {
			return i, true
		}
	}
	return -1, false
}

func collectCFNDeps(v any, self string, resources map[string]cfnResource, out map[string]struct{}) {
	switch x := v.(type) {
	case map[string]any:
		if ref, ok := x["Ref"]; ok {
			name := fmt.Sprint(ref)
			if _, exists := resources[name]; exists && name != self {
				out[name] = struct{}{}
			}
		}
		if ga, ok := x["Fn::GetAtt"]; ok {
			logicalID, _, err := parseGetAtt(ga)
			if err == nil {
				if _, exists := resources[logicalID]; exists && logicalID != self {
					out[logicalID] = struct{}{}
				}
			}
		}
		if sub, ok := x["Fn::Sub"]; ok {
			collectSubDeps(sub, self, resources, out)
		}
		for _, vv := range x {
			collectCFNDeps(vv, self, resources, out)
		}
	case []any:
		for _, item := range x {
			collectCFNDeps(item, self, resources, out)
		}
	}
}

func collectSubDeps(v any, self string, resources map[string]cfnResource, out map[string]struct{}) {
	switch t := v.(type) {
	case string:
		for _, m := range cfnSubToken.FindAllStringSubmatch(t, -1) {
			name := m[1]
			if strings.Contains(name, ".") {
				name = strings.SplitN(name, ".", 2)[0]
			}
			if _, exists := resources[name]; exists && name != self {
				out[name] = struct{}{}
			}
		}
	case []any:
		if len(t) > 0 {
			if s, ok := t[0].(string); ok {
				collectSubDeps(s, self, resources, out)
			}
		}
		if len(t) > 1 {
			collectCFNDeps(t[1], self, resources, out)
		}
	}
}

func cfnStringProp(props map[string]any, key string) string {
	v, ok := props[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case int:
		return strconv.Itoa(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func cfnIntProp(props map[string]any, key string, fallback int) int {
	v, ok := props[key]
	if !ok || v == nil {
		return fallback
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return fallback
		}
		return n
	default:
		return fallback
	}
}

func supportedCFNType(t string) bool {
	switch t {
	case "AWS::S3::Bucket", "AWS::IAM::Role", "AWS::SQS::Queue", "AWS::DynamoDB::Table", "AWS::Lambda::Function":
		return true
	default:
		return false
	}
}
