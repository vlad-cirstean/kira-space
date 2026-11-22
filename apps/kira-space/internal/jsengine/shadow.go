package jsengine

import (
	"reflect"
	"slices"

	js_ast "github.com/dop251/goja/ast"
)

// declares returns the first of names that the program binds with a declaration (var, let, const,
// function, class, parameter or catch binding, patterns included), or "". A script that does so
// would shadow a host global silently.
func declares(prog *js_ast.Program, names []string) string {
	var found string
	hit := func(id *js_ast.Identifier) {
		if id != nil && found == "" && slices.Contains(names, id.Name.String()) {
			found = id.Name.String()
		}
	}
	var target func(e js_ast.Expression)
	target = func(e js_ast.Expression) {
		switch t := e.(type) {
		case *js_ast.Identifier:
			hit(t)
		case *js_ast.ObjectPattern:
			for _, p := range t.Properties {
				switch p := p.(type) {
				case *js_ast.PropertyShort:
					hit(&p.Name)
				case *js_ast.PropertyKeyed:
					target(p.Value)
				}
			}
			if t.Rest != nil {
				target(t.Rest)
			}
		case *js_ast.ArrayPattern:
			for _, el := range t.Elements {
				target(el)
			}
			if t.Rest != nil {
				target(t.Rest)
			}
		case *js_ast.AssignExpression: // default value inside a pattern
			target(t.Left)
		}
	}
	visit(reflect.ValueOf(prog), func(n any) {
		switch n := n.(type) {
		case *js_ast.Binding:
			target(n.Target)
		case *js_ast.FunctionLiteral:
			hit(n.Name)
		case *js_ast.ClassLiteral:
			hit(n.Name)
		case *js_ast.ParameterList:
			if n.Rest != nil {
				target(n.Rest)
			}
		case *js_ast.CatchStatement:
			if n.Parameter != nil {
				target(n.Parameter)
			}
		case *js_ast.ForDeclaration:
			target(n.Target)
		}
	})
	return found
}

var nodeIface = reflect.TypeFor[js_ast.Node]()

// visit walks every AST node reachable from v, calling fn on each pointer to a node struct.
func visit(v reflect.Value, fn func(any)) {
	switch v.Kind() {
	case reflect.Interface:
		if !v.IsNil() {
			visit(v.Elem(), fn)
		}
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		if v.Type().Implements(nodeIface) || v.Type() == reflect.TypeFor[*js_ast.Binding]() || v.Type() == reflect.TypeFor[*js_ast.ParameterList]() {
			fn(v.Interface())
		}
		visit(v.Elem(), fn)
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				visit(v.Field(i), fn)
			}
		}
	case reflect.Slice:
		for i := range v.Len() {
			visit(v.Index(i), fn)
		}
	}
}
