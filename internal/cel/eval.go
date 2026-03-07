package cel

import (
	"regexp"

	gcel "github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/ref"
)

// loopIdentRe matches the identifier "loop" when followed by a dot, ensuring it
// is not preceded by a word character (i.e. it is a standalone identifier).
// This rewrites developer-facing "loop.x" references to the internal "_loop.x"
// name, since "loop" is a reserved keyword in the CEL grammar.
var loopIdentRe = regexp.MustCompile(`\bloop\.`)

// rewriteLoopIdent transparently translates "loop." to "_loop." in an expression
// so that workflow authors can write natural "loop.index", "loop.first", etc.
// without knowing about the CEL reserved-identifier constraint.
func rewriteLoopIdent(expr string) string {
	return loopIdentRe.ReplaceAllString(expr, "_loop.")
}

// Compile parses and type-checks the CEL expression, returning a compiled Program
// ready for repeated evaluation. Call this at parse/lint time to surface
// syntax and type errors early.
//
// Occurrences of "loop." in expr are automatically rewritten to "_loop." before
// compilation because "loop" is a reserved identifier in the CEL grammar.
func (e *Environment) Compile(expr string) (gcel.Program, error) {
	rewritten := rewriteLoopIdent(expr)

	e.mu.RLock()
	if prog, ok := e.programs[rewritten]; ok {
		e.mu.RUnlock()
		return prog, nil
	}
	e.mu.RUnlock()

	ast, issues := e.env.Compile(rewritten)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	prg, err := e.env.Program(ast)
	if err != nil {
		return nil, err
	}

	e.mu.Lock()
	if cached, ok := e.programs[rewritten]; ok {
		e.mu.Unlock()
		return cached, nil
	}
	e.programs[rewritten] = prg
	e.mu.Unlock()

	return prg, nil
}

// Eval evaluates a pre-compiled Program against the given runtime State and
// returns the result as a native Go value. Type coercion is handled by the
// CEL runtime; the result is unwrapped to its underlying Go representation.
func (e *Environment) Eval(prog gcel.Program, s *State) (any, error) {
	out, _, err := prog.Eval(e.Bindings(s))
	if err != nil {
		return nil, err
	}
	return nativeValue(out), nil
}

// nativeValue converts a CEL ref.Val to its underlying native Go representation
// by calling the value's Value() method.
func nativeValue(v ref.Val) any {
	if v == nil {
		return nil
	}
	return v.Value()
}
