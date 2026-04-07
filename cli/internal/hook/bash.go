package hook

import (
	"encoding/json"
	"regexp"
	"strings"
)

type bashToolInput struct {
	Command string `json:"command"`
	Cmd     string `json:"cmd"`
}

var (
	// reRedirect matches stdout redirections: > file or >> file.
	// Captures the destination path.
	//
	// Deliberately excludes:
	//   2> file   (stderr redirect — digit immediately before >)
	//   &> file   (combined redirect — & immediately before >)
	//   >&2       (redirect-to-descriptor — > followed by &)
	//
	// Matches:
	//   > file, >> file                (bare redirect)
	//   1> file, 1>> file              (explicit stdout)
	//   echo foo > file                (after command)
	//   cmd1 | cmd2 > file             (after pipe)
	//   cmd; cmd2 >> file              (after semicolon)
	reRedirect = regexp.MustCompile(`(?:^|[ \t;|&])(?:1?)>>?[ \t]+([^\s;&|><'"` + "`" + `]+)`)

	// reTee matches: tee <file> and tee -a <file>
	reTee = regexp.MustCompile(`\btee\b(?:\s+-a)?\s+([^\s;&|]+)`)

	// reCpMv matches cp/mv with a destination as the last whitespace-separated
	// token. Handles: cp src dst, mv src dst, cp -r src/ dst/.
	reCpMv = regexp.MustCompile(`\b(?:cp|mv)\b\s+(?:[^\s;&|]+\s+)+([^\s;&|]+)`)
)

// bashWriteTargets parses a bash command string and returns all file paths
// that the command would write to. Returns nil if no write patterns are found.
//
// Recognised patterns:
//
//	> file, >> file      stdout redirections (create or append)
//	tee [-a] file        tee command
//	sed -i ... file      in-place edit (last positional argument)
//	cp src dst           copy — destination path
//	mv src dst           move — destination path
func bashWriteTargets(command string) []string {
	seen := map[string]bool{}
	var targets []string

	add := func(path string) {
		path = strings.Trim(strings.TrimSpace(path), `'"`)
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		targets = append(targets, path)
	}

	for _, m := range reRedirect.FindAllStringSubmatch(command, -1) {
		add(m[1])
	}

	for _, m := range reTee.FindAllStringSubmatch(command, -1) {
		add(m[1])
	}

	for _, m := range reSedInPlaceTargets(command) {
		add(m)
	}

	for _, m := range reCpMv.FindAllStringSubmatch(command, -1) {
		add(m[1])
	}

	return targets
}

// reSedInPlaceTargets extracts target file paths from sed -i invocations.
// Uses tokenisation rather than a single regex because sed's flag ordering
// and optional suffix (e.g. -i.bak) make a correct single regex unwieldy.
func reSedInPlaceTargets(command string) []string {
	if !strings.Contains(command, "sed") {
		return nil
	}

	tokens := strings.Fields(command)
	sedIdx := -1
	for i, t := range tokens {
		if t == "sed" {
			sedIdx = i
			break
		}
	}
	if sedIdx == -1 {
		return nil
	}

	// Look for -i flag anywhere after sed
	hasInPlace := false
	for _, t := range tokens[sedIdx+1:] {
		if t == "-i" || strings.HasPrefix(t, "-i") {
			hasInPlace = true
			break
		}
	}
	if !hasInPlace {
		return nil
	}

	// The target file is the last token that does not start with - or ;|&
	for i := len(tokens) - 1; i > sedIdx; i-- {
		t := tokens[i]
		if !strings.HasPrefix(t, "-") && !strings.ContainsAny(t, ";&|") {
			return []string{t}
		}
	}
	return nil
}

// bashReadTargets parses a bash command string and returns file paths that the
// command would read. Only simple, unambiguous read patterns are detected;
// the goal is defence-in-depth for prevent-read file rules (the Read tool
// block is the primary enforcement path).
//
// Recognised patterns:
//
//	cat file [file…]
//	head [-n N] file
//	tail [-n N] file
//	less file
//	more file
func bashReadTargets(command string) []string {
	seen := map[string]bool{}
	var targets []string

	add := func(path string) {
		path = strings.Trim(strings.TrimSpace(path), `'"`)
		if path == "" || seen[path] || strings.HasPrefix(path, "-") {
			return
		}
		seen[path] = true
		targets = append(targets, path)
	}

	tokens := strings.Fields(command)
	for i, tok := range tokens {
		switch tok {
		case "cat":
			// cat [options] file… — collect non-flag tokens until a shell delimiter
			for _, t := range tokens[i+1:] {
				if isShellDelimiter(t) {
					break
				}
				if strings.HasPrefix(t, "-") {
					continue
				}
				add(t)
			}
		case "head", "tail":
			// head/tail [-n N | -N] file
			j := i + 1
			for j < len(tokens) {
				t := tokens[j]
				if t == "-n" {
					j += 2 // skip flag and its value
					continue
				}
				if strings.HasPrefix(t, "-") {
					j++
					continue
				}
				add(t)
				break
			}
		case "less", "more":
			// less/more [options] file
			for _, t := range tokens[i+1:] {
				if strings.HasPrefix(t, "-") {
					continue
				}
				add(t)
				break
			}
		}
	}

	return targets
}

// isShellDelimiter reports whether token is a shell operator that separates
// commands (|, ||, &&, ;). Used to stop collecting file arguments for a
// command when the rest of the token stream belongs to a different command.
func isShellDelimiter(token string) bool {
	switch token {
	case "|", "||", "&&", ";":
		return true
	}
	return false
}

// parseBashToolInput extracts the command string from a Bash tool_input JSON blob.
// Returns empty string if the field is missing or the JSON is malformed.
func parseBashToolInput(raw json.RawMessage) string {
	var inp bashToolInput
	if err := json.Unmarshal([]byte(raw), &inp); err != nil {
		return ""
	}
	if inp.Command != "" {
		return inp.Command
	}
	return inp.Cmd
}
