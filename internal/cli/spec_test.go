package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type specNode struct {
	flags    map[string]bool
	aliases  map[string]bool
	children map[string]*specNode
}

func newSpecNode() *specNode {
	return &specNode{
		flags:    map[string]bool{},
		aliases:  map[string]bool{},
		children: map[string]*specNode{},
	}
}

func TestUsageSpecParity(t *testing.T) {
	spec, err := parseUsageSpecFile("../../watchtower.usage.kdl")
	if err != nil {
		t.Fatal(err)
	}
	diffs := diffSurface("watchtower-cli", spec, cobraSurface(NewRoot()))
	if len(diffs) > 0 {
		t.Fatalf("usage spec drifted from cobra:\n%s", strings.Join(diffs, "\n"))
	}
}

func parseUsageSpecFile(path string) (*specNode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseUsageSpec(string(data))
}

func parseUsageSpec(text string) (*specNode, error) {
	root := newSpecNode()
	stack := []*specNode{root}
	for i, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}
		cur := stack[len(stack)-1]
		delta := braceDelta(trimmed)
		switch {
		case strings.HasPrefix(trimmed, "flag "):
			name := longFlag(trimmed)
			if name == "" {
				return nil, fmt.Errorf("line %d: flag has no long name: %s", i+1, trimmed)
			}
			cur.flags[name] = true
		case strings.HasPrefix(trimmed, "cmd "):
			name := quotedName(trimmed)
			if name == "" {
				return nil, fmt.Errorf("line %d: cmd has no name: %s", i+1, trimmed)
			}
			child := newSpecNode()
			cur.children[name] = child
			if delta > 0 {
				stack = append(stack, child)
				delta--
			}
		case strings.HasPrefix(trimmed, "alias "):
			name := quotedName(trimmed)
			if name == "" {
				return nil, fmt.Errorf("line %d: alias has no name: %s", i+1, trimmed)
			}
			cur.aliases[name] = true
		}
		for delta < 0 {
			if len(stack) == 1 {
				return nil, fmt.Errorf("line %d: extra closing brace", i+1)
			}
			stack = stack[:len(stack)-1]
			delta++
		}
		if delta > 0 {
			return nil, fmt.Errorf("line %d: unexpected brace", i+1)
		}
	}
	if len(stack) != 1 {
		return nil, fmt.Errorf("unclosed command block")
	}
	return root, nil
}

func longFlag(line string) string {
	head := line
	if i := strings.Index(line, " help="); i >= 0 {
		head = line[:i]
	}
	i := strings.Index(head, "--")
	if i < 0 {
		return ""
	}
	rest := head[i+2:]
	end := 0
	for end < len(rest) {
		switch rest[end] {
		case ' ', '"', '<', '=':
			return rest[:end]
		}
		end++
	}
	return rest
}

func quotedName(line string) string {
	i := strings.IndexByte(line, '"')
	if i < 0 {
		return ""
	}
	rest := line[i+1:]
	j := strings.IndexByte(rest, '"')
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func braceDelta(line string) int {
	inQuote := false
	delta := 0
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == '"' && (i == 0 || line[i-1] != '\\') {
			inQuote = !inQuote
			continue
		}
		if inQuote {
			continue
		}
		switch c {
		case '{':
			delta++
		case '}':
			delta--
		}
	}
	return delta
}

func cobraSurface(cmd *cobra.Command) *specNode {
	n := newSpecNode()
	flags := cmd.LocalFlags()
	if cmd.Parent() == nil {
		flags = cmd.PersistentFlags()
	}
	flags.VisitAll(func(f *pflag.Flag) {
		n.flags[f.Name] = true
	})
	for _, alias := range cmd.Aliases {
		n.aliases[alias] = true
	}
	for _, child := range cmd.Commands() {
		if child.Name() == "help" || child.Hidden {
			continue
		}
		n.children[child.Name()] = cobraSurface(child)
	}
	return n
}

func diffSurface(path string, spec, got *specNode) []string {
	var diffs []string
	diffs = append(diffs, diffNames(path, "flag", spec.flags, got.flags)...)
	diffs = append(diffs, diffNames(path, "alias", spec.aliases, got.aliases)...)
	diffs = append(diffs, diffNames(path, "command", childNames(spec.children), childNames(got.children))...)
	for _, name := range sortedKeys(childNames(spec.children)) {
		if spec.children[name] == nil || got.children[name] == nil {
			continue
		}
		childPath := path + " " + name
		diffs = append(diffs, diffSurface(childPath, spec.children[name], got.children[name])...)
	}
	return diffs
}

func childNames(children map[string]*specNode) map[string]bool {
	names := make(map[string]bool, len(children))
	for name := range children {
		names[name] = true
	}
	return names
}

func diffNames(path, kind string, spec, got map[string]bool) []string {
	var diffs []string
	for _, name := range sortedKeys(spec) {
		if !got[name] {
			diffs = append(diffs, fmt.Sprintf("%s: missing %s %s", path, kind, name))
		}
	}
	for _, name := range sortedKeys(got) {
		if !spec[name] {
			diffs = append(diffs, fmt.Sprintf("%s: extra %s %s", path, kind, name))
		}
	}
	return diffs
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
