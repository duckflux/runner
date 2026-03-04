package cel

import (
	gcel "github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/ref"
)

// Compile parses and type-checks the CEL expression, returning a compiled Program
// ready for repeated evaluation. Call this at parse/lint time to surface
// syntax and type errors early.
func (e *Environment) Compile(expr string) (gcel.Program, error) {
	ast, issues := e.env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	prg, err := e.env.Program(ast)
	if err != nil {
		return nil, err
	}
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
