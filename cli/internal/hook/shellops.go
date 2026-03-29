package hook

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

const (
	shellOpExec     = "exec"
	shellOpRead     = "read"
	shellOpMutation = "mutation"
)

type shellOp struct {
	Type    string `json:"type"`
	Command string `json:"command,omitempty"`
	Path    string `json:"path,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
	Source  string `json:"source,omitempty"`
}

type shellAnalysis struct {
	CommandRaw    string
	ParsedOK      bool
	ParseError    string
	Parser        string
	ParserVersion string
	Ambiguity     []string
	Commands      []string
	Ops           []shellOp
	EffectiveCwd  string
}

func (a shellAnalysis) ambiguityText() string {
	if len(a.Ambiguity) == 0 {
		return ""
	}
	return strings.Join(a.Ambiguity, ";")
}

func (a shellAnalysis) opsJSON() string {
	if len(a.Ops) == 0 {
		return "[]"
	}
	b, err := json.Marshal(a.Ops)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func analyzeShellCommand(command, cwd string) shellAnalysis {
	a := shellAnalysis{
		CommandRaw:    strings.TrimSpace(command),
		Parser:        "mvdan.cc/sh/v3/syntax",
		ParserVersion: "v3",
		EffectiveCwd:  cwd,
	}

	a.Commands = splitCompoundCommand(command)

	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	if _, err := parser.Parse(strings.NewReader(command), ""); err != nil {
		a.ParseError = err.Error()
		a.Ambiguity = append(a.Ambiguity, "parse_error")
	} else {
		a.ParsedOK = true
	}

	effectiveCwd := cwd
	for _, seg := range a.Commands {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}

		a.Ops = append(a.Ops, shellOp{Type: shellOpExec, Command: seg, Cwd: effectiveCwd, Source: "parser"})

		argv, ok := parseShellArgv(seg)
		if !ok || len(argv) == 0 {
			a.Ambiguity = append(a.Ambiguity, "argv_unresolved")
			// Fall back to legacy heuristics for targets in this segment.
			for _, p := range bashReadTargets(seg) {
				a.Ops = append(a.Ops, shellOp{
					Type:   shellOpRead,
					Path:   resolveShellPath(p, effectiveCwd),
					Cwd:    effectiveCwd,
					Source: "legacy_read",
				})
			}
			for _, p := range bashWriteTargets(seg) {
				a.Ops = append(a.Ops, shellOp{
					Type:   shellOpMutation,
					Path:   resolveShellPath(p, effectiveCwd),
					Cwd:    effectiveCwd,
					Source: "legacy_write",
				})
			}
			continue
		}

		cmd := argv[0]
		low := strings.ToLower(cmd)

		if low == "cd" {
			if len(argv) > 1 {
				effectiveCwd = resolveShellPath(argv[1], effectiveCwd)
			} else {
				a.Ambiguity = append(a.Ambiguity, "cd_no_target")
			}
			continue
		}

		a.Ops = append(a.Ops, extractOpsFromArgv(argv, effectiveCwd)...)

		// Preserve broad write/read detection coverage for shell syntax patterns
		// not yet represented in command-specific extraction logic.
		for _, p := range bashReadTargets(seg) {
			a.Ops = append(a.Ops, shellOp{
				Type:   shellOpRead,
				Path:   resolveShellPath(p, effectiveCwd),
				Cwd:    effectiveCwd,
				Source: "legacy_read",
			})
		}
		for _, p := range bashWriteTargets(seg) {
			a.Ops = append(a.Ops, shellOp{
				Type:   shellOpMutation,
				Path:   resolveShellPath(p, effectiveCwd),
				Cwd:    effectiveCwd,
				Source: "legacy_write",
			})
		}
	}

	a.EffectiveCwd = effectiveCwd
	a.Ops = dedupeOps(a.Ops)
	return a
}

func parseShellArgv(seg string) ([]string, bool) {
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	f, err := parser.Parse(strings.NewReader(seg), "")
	if err != nil || len(f.Stmts) == 0 {
		return nil, false
	}
	call, ok := f.Stmts[0].Cmd.(*syntax.CallExpr)
	if !ok {
		return nil, false
	}
	if len(call.Args) == 0 {
		return nil, false
	}
	argv := make([]string, 0, len(call.Args))
	for _, w := range call.Args {
		v, resolved := wordToString(w)
		if strings.TrimSpace(v) == "" {
			continue
		}
		argv = append(argv, v)
		if !resolved {
			return argv, false
		}
	}
	return argv, true
}

func wordToString(w *syntax.Word) (string, bool) {
	var b strings.Builder
	resolved := true
	for _, p := range w.Parts {
		switch x := p.(type) {
		case *syntax.Lit:
			b.WriteString(x.Value)
		case *syntax.SglQuoted:
			b.WriteString(x.Value)
		case *syntax.DblQuoted:
			for _, qp := range x.Parts {
				switch y := qp.(type) {
				case *syntax.Lit:
					b.WriteString(y.Value)
				default:
					resolved = false
				}
			}
		default:
			// Parameter expansion, command substitution, arithmetic, etc.
			resolved = false
		}
	}
	if resolved {
		return b.String(), true
	}
	// Best-effort fallback preserving original text.
	var buf bytes.Buffer
	_ = syntax.NewPrinter().Print(&buf, w)
	return strings.TrimSpace(buf.String()), false
}

func extractOpsFromArgv(argv []string, cwd string) []shellOp {
	if len(argv) == 0 {
		return nil
	}
	cmd := strings.ToLower(argv[0])
	var ops []shellOp

	switch cmd {
	case "cat", "head", "tail", "less", "more":
		for _, p := range nonFlagPaths(argv[1:]) {
			ops = append(ops, shellOp{Type: shellOpRead, Path: resolveShellPath(p, cwd), Cwd: cwd, Source: "argv"})
		}
	case "git":
		if len(argv) < 2 {
			return ops
		}
		sub := strings.ToLower(argv[1])
		switch sub {
		case "add":
			for _, p := range nonFlagPaths(argv[2:]) {
				ops = append(ops, shellOp{Type: shellOpMutation, Path: resolveShellPath(p, cwd), Cwd: cwd, Source: "argv"})
			}
		case "commit":
			if hasAnyFlag(argv[2:], "-a", "--all") {
				ops = append(ops, shellOp{Type: shellOpMutation, Path: resolveShellPath(".", cwd), Cwd: cwd, Source: "argv"})
			}
		}
	case "cp":
		args := nonFlagPaths(argv[1:])
		if len(args) >= 2 {
			dst := args[len(args)-1]
			ops = append(ops, shellOp{Type: shellOpMutation, Path: resolveShellPath(dst, cwd), Cwd: cwd, Source: "argv"})
		}
	case "mv":
		args := nonFlagPaths(argv[1:])
		if len(args) >= 2 {
			dst := args[len(args)-1]
			ops = append(ops, shellOp{Type: shellOpMutation, Path: resolveShellPath(dst, cwd), Cwd: cwd, Source: "argv"})
			for _, src := range args[:len(args)-1] {
				ops = append(ops, shellOp{Type: shellOpMutation, Path: resolveShellPath(src, cwd), Cwd: cwd, Source: "argv"})
			}
		}
	case "rm", "touch", "mkdir":
		for _, p := range nonFlagPaths(argv[1:]) {
			ops = append(ops, shellOp{Type: shellOpMutation, Path: resolveShellPath(p, cwd), Cwd: cwd, Source: "argv"})
		}
	case "tee":
		args := nonFlagPaths(argv[1:])
		if len(args) > 0 {
			ops = append(ops, shellOp{Type: shellOpMutation, Path: resolveShellPath(args[0], cwd), Cwd: cwd, Source: "argv"})
		}
	case "sed":
		if hasSedInPlace(argv[1:]) {
			args := nonFlagPaths(argv[1:])
			if len(args) > 0 {
				target := args[len(args)-1]
				ops = append(ops, shellOp{Type: shellOpMutation, Path: resolveShellPath(target, cwd), Cwd: cwd, Source: "argv"})
			}
		}
	}
	return ops
}

func nonFlagPaths(args []string) []string {
	var paths []string
	stopFlags := false
	for _, a := range args {
		if a == "--" {
			stopFlags = true
			continue
		}
		if !stopFlags && strings.HasPrefix(a, "-") {
			continue
		}
		if strings.TrimSpace(a) == "" {
			continue
		}
		paths = append(paths, a)
	}
	return paths
}

func hasAnyFlag(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f {
				return true
			}
		}
	}
	return false
}

func hasSedInPlace(args []string) bool {
	for _, a := range args {
		if a == "-i" || strings.HasPrefix(a, "-i") {
			return true
		}
	}
	return false
}

func resolveShellPath(pathArg, cwd string) string {
	pathArg = strings.Trim(strings.TrimSpace(pathArg), `"'`)
	if pathArg == "" {
		return ""
	}
	if filepath.IsAbs(pathArg) {
		return filepath.Clean(pathArg)
	}
	if cwd == "" {
		return filepath.Clean(pathArg)
	}
	return filepath.Clean(filepath.Join(cwd, pathArg))
}

func dedupeOps(ops []shellOp) []shellOp {
	seen := map[string]bool{}
	var out []shellOp
	for _, op := range ops {
		if op.Type == "" {
			continue
		}
		key := op.Type + "|" + op.Command + "|" + op.Path + "|" + op.Cwd + "|" + op.Source
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, op)
	}
	return out
}
