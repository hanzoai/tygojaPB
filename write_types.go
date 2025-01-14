package tygojaPB

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"go/ast"
	"go/token"
)

// Options for the writeType() method that can be used for extra context
// to determine the format of the return type.
const (
	optionExtends        = "extends"
	optionParenthesis    = "parenthesis"
	optionFunctionReturn = "func_return"
)

func (g *PackageGenerator) writeIndent(s *strings.Builder, depth int) {
	for i := 0; i < depth; i++ {
		s.WriteString(g.conf.Indent)
	}
}

func (g *PackageGenerator) writeStartModifier(s *strings.Builder, depth int) {
	g.writeIndent(s, depth)

	if g.conf.StartModifier != "" {
		s.WriteString(g.conf.StartModifier)
		s.WriteString(" ")
	}
}

func (g *PackageGenerator) writeType(s *strings.Builder, t ast.Expr, depth int, options ...string) {
	switch t := t.(type) {
	case *ast.StarExpr:
		if hasOption(optionParenthesis, options) {
			s.WriteByte('(')
		}

		g.writeType(s, t.X, depth)

		// allow undefined union only when not used in an "extends" expression or as return type
		if !hasOption(optionExtends, options) && !hasOption(optionFunctionReturn, options) {
			s.WriteString(" | undefined")
		}

		if hasOption(optionParenthesis, options) {
			s.WriteByte(')')
		}
	case *ast.Ellipsis:
		if v, ok := t.Elt.(*ast.Ident); ok && v.String() == "byte" {
			s.WriteString("string")
			break
		}

		// wrap variadic args with parenthesis to support function declarations
		// (eg. "...callbacks: (() => number)[]")
		_, isFunc := t.Elt.(*ast.FuncType)
		if isFunc {
			s.WriteString("(")
		}

		g.writeType(s, t.Elt, depth, optionParenthesis)

		if isFunc {
			s.WriteString(")")
		}

		s.WriteString("[]")
	case *ast.ArrayType:
		if v, ok := t.Elt.(*ast.Ident); ok && v.String() == "byte" && !hasOption(optionExtends, options) {
			// union type with string since depending where it is used
			// goja auto converts string to []byte if the field expect that
			s.WriteString("string|")
		}

		s.WriteString("Array<")
		g.writeType(s, t.Elt, depth, optionParenthesis)
		s.WriteString(">")
	case *ast.StructType:
		s.WriteString("{\n")
		g.writeStructFields(s, t.Fields.List, depth+1)
		g.writeIndent(s, depth+1)
		s.WriteByte('}')
	case *ast.Ident:
		v := t.String()

		mappedType, ok := g.conf.TypeMappings[v]
		if ok {
			// use the mapped type
			v = mappedType
		} else {
			// try to find a matching js equivalent
			switch v {
			case "string":
				v = "string"
			case "bool":
				v = "boolean"
			case "int", "int8", "int16", "int32", "int64",
				"uint", "uint8", "uint16", "uint32", "uint64",
				"float32", "float64",
				"complex64", "complex128",
				"uintptr", "byte", "rune":
				v = "number"
			case "error":
				v = "Error"
			default:
				g.unknownTypes[v] = struct{}{}
			}
		}

		s.WriteString(v)
	case *ast.SelectorExpr:
		// e.g. `unsafe.Pointer` or `unsafe.*`
		fullType := fmt.Sprintf("%s.%s", t.X, t.Sel)
		fullTypeWildcard := fmt.Sprintf("%s.*", t.X)

		if v, ok := g.conf.TypeMappings[fullType]; ok {
			s.WriteString(v)
		} else if v, ok := g.conf.TypeMappings[fullTypeWildcard]; ok {
			s.WriteString(v)
		} else {
			g.unknownTypes[fullType] = struct{}{}
			s.WriteString(fullType)
		}
	case *ast.MapType:
		s.WriteString("_TygojaDict")
	case *ast.BasicLit:
		s.WriteString(t.Value)
	case *ast.ParenExpr:
		s.WriteByte('(')
		g.writeType(s, t.X, depth)
		s.WriteByte(')')
	case *ast.BinaryExpr:
		g.writeType(s, t.X, depth)
		s.WriteByte(' ')
		s.WriteString(t.Op.String())
		s.WriteByte(' ')
		g.writeType(s, t.Y, depth)
	case *ast.InterfaceType:
		s.WriteString("{\n")
		g.writeInterfaceFields(s, t.Methods.List, depth)
		g.writeIndent(s, depth+1)
		s.WriteByte('}')
	case *ast.FuncType:
		g.writeFuncType(s, t, depth, hasOption(optionParenthesis, options))
	case *ast.UnaryExpr:
		if t.Op == token.TILDE {
			// we just ignore the tilde token, in Typescript extended types are
			// put into the generic typing itself, which we can't support yet.
			g.writeType(s, t.X, depth)
		} else {
			// only log for now
			log.Printf("unhandled unary expr: %v\n %T\n", t, t)
		}
	case *ast.IndexListExpr:
		g.writeType(s, t.X, depth)
		s.WriteByte('<')
		for i, index := range t.Indices {
			g.writeType(s, index, depth)
			if i != len(t.Indices)-1 {
				s.WriteString(", ")
			}
		}
		s.WriteByte('>')
	case *ast.IndexExpr:
		g.writeType(s, t.X, depth)
		s.WriteByte('<')
		g.writeType(s, t.Index, depth)
		s.WriteByte('>')
	case *ast.CallExpr, *ast.ChanType, *ast.CompositeLit:
		s.WriteString("undefined")
	default:
		s.WriteString("any")
	}
}

func (g *PackageGenerator) writeTypeParamsFields(s *strings.Builder, fields []*ast.Field) {
	// extract params
	names := []string{}
	for _, f := range fields {
		for _, ident := range f.Names {
			names = append(names, ident.Name)

			// disable extends for now as it complicates the interfaces merge
			//
			// s.WriteString(" extends ")
			// g.writeType(s, f.Type, 0, true)
			// if i != len(fields)-1 || j != len(f.Names)-1 {
			// 	s.WriteString(", ")
			// }
		}
	}

	if len(names) == 0 {
		return
	}

	s.WriteByte('<')

	for i, name := range names {
		if i > 0 {
			s.WriteString(",")
		}
		s.WriteString(name)
	}

	s.WriteByte('>')
}

func (g *PackageGenerator) writeInterfaceFields(s *strings.Builder, fields []*ast.Field, depth int) {
	for _, f := range fields {
		g.writeCommentGroup(s, f.Doc, depth+1)

		var methodName string
		if len(f.Names) != 0 && f.Names[0] != nil && len(f.Names[0].Name) != 0 {
			methodName = f.Names[0].Name
		}
		if len(methodName) == 0 || 'A' > methodName[0] || methodName[0] > 'Z' {
			continue
		}

		if g.conf.MethodNameFormatter != nil {
			methodName = g.conf.MethodNameFormatter(methodName)
		}

		g.writeIndent(s, depth+1)
		s.WriteString(methodName)
		g.writeType(s, f.Type, depth)

		if f.Comment != nil {
			s.WriteString(" // ")
			s.WriteString(f.Comment.Text())
		} else {
			s.WriteByte('\n')
		}
	}
}

func (g *PackageGenerator) writeStructFields(s *strings.Builder, fields []*ast.Field, depth int) {
	for _, f := range fields {
		var fieldName string
		if len(f.Names) != 0 && f.Names[0] != nil && len(f.Names[0].Name) != 0 {
			fieldName = f.Names[0].Name
		}
		if len(fieldName) == 0 || 'A' > fieldName[0] || fieldName[0] > 'Z' {
			continue
		}

		if g.conf.FieldNameFormatter != nil {
			fieldName = g.conf.FieldNameFormatter(fieldName)
		}

		g.writeCommentGroup(s, f.Doc, depth+1)

		g.writeIndent(s, depth+1)
		quoted := !isValidJSName(fieldName)
		if quoted {
			s.WriteByte('\'')
		}
		s.WriteString(fieldName)
		if quoted {
			s.WriteByte('\'')
		}

		// check if it is nil-able, aka. optional
		switch t := f.Type.(type) {
		case *ast.StarExpr:
			f.Type = t.X
			s.WriteByte('?')
		}

		s.WriteString(": ")
		g.writeType(s, f.Type, depth, optionParenthesis)

		if f.Comment != nil {
			// Line comment is present, that means a comment after the field.
			s.WriteString(" // ")
			s.WriteString(f.Comment.Text())
		} else {
			s.WriteByte('\n')
		}
	}
}

func (g *PackageGenerator) writeFuncType(s *strings.Builder, t *ast.FuncType, depth int, returnAsProp bool) {
	s.WriteString("(")

	if t.Params != nil {
		g.writeFuncParams(s, t.Params.List, depth)
	}

	if returnAsProp {
		s.WriteString(") => ")
	} else {
		s.WriteString("): ")
	}

	// (from https://pkg.go.dev/github.com/dop251/goja)
	// Functions with multiple return values return an Array.
	// If the last return value is an `error` it is not returned but converted into a JS exception.
	// If the error is *Exception, it is thrown as is, otherwise it's wrapped in a GoEerror.
	// Note that if there are exactly two return values and the last is an `error`,
	// the function returns the first value as is, not an Array.
	if t.Results == nil || len(t.Results.List) == 0 {
		s.WriteString("void")
	} else {
		// remove the last return error type
		lastReturn, ok := t.Results.List[len(t.Results.List)-1].Type.(*ast.Ident)
		if ok && lastReturn.Name == "error" {
			t.Results.List = t.Results.List[:len(t.Results.List)-1]
		}

		if len(t.Results.List) == 0 {
			s.WriteString("void")
		} else {
			// multiple and shortened return type values must be wrapped in []
			// (combined/shortened return values from the same type are part of a single ast.Field but with different names)
			hasMultipleReturnValues := len(t.Results.List) > 1 || len(t.Results.List[0].Names) > 1
			if hasMultipleReturnValues {
				s.WriteRune('[')
			}

			for i, f := range t.Results.List {
				totalNames := max(len(f.Names), 1)
				for j := range totalNames {
					if i > 0 || j > 0 {
						s.WriteString(", ")
					}

					g.writeType(s, f.Type, 0, optionParenthesis, optionFunctionReturn)
				}
			}

			if hasMultipleReturnValues {
				s.WriteRune(']')
			}
		}
	}
}

func (g *PackageGenerator) writeFuncParams(s *strings.Builder, params []*ast.Field, depth int) {
	for i, f := range params {
		// normalize params iteration
		// (params with omitted types will be part of a single ast.Field but with different names)
		names := make([]string, 0, len(f.Names))
		for j, ident := range f.Names {
			name := ident.Name
			if name == "" || isReservedIdentifier(name) {
				name = fmt.Sprintf("_arg%d%d", i, j)
			}
			names = append(names, name)
		}
		if len(names) == 0 {
			// ommitted param name (eg. func(string))
			names = append(names, fmt.Sprintf("_arg%d", i))
		}

		for j, fieldName := range names {
			if i > 0 || j > 0 {
				s.WriteString(", ")
			}

			var isVariadic bool

			switch t := f.Type.(type) {
			case *ast.StarExpr:
				f.Type = t.X
			case *ast.Ellipsis:
				isVariadic = true
			}

			g.writeCommentGroup(s, f.Doc, depth+2)
			if isVariadic {
				s.WriteString("...")
			}
			s.WriteString(fieldName)

			s.WriteString(": ")

			g.writeType(s, f.Type, depth, optionParenthesis)

			if f.Comment != nil {
				// Line comment is present, that means a comment after the field.
				s.WriteString(" /* ")
				s.WriteString(f.Comment.Text())
				s.WriteString(" */ ")
			}
		}
	}
}

// see https://es5.github.io/#x7.6.1.1
var reservedIdentifiers = map[string]struct{}{
	"break":      {},
	"do":         {},
	"instanceof": {},
	"typeof":     {},
	"case":       {},
	"else":       {},
	"new":        {},
	"var":        {},
	"catch":      {},
	"finally":    {},
	"return":     {},
	"void":       {},
	"continue":   {},
	"for":        {},
	"switch":     {},
	"while":      {},
	"debugger":   {},
	"function":   {},
	"this":       {},
	"with":       {},
	"default":    {},
	"if":         {},
	"throw":      {},
	"delete":     {},
	"in":         {},
	"try":        {},
	"class":      {},
	"enum":       {},
	"extends":    {},
	"super":      {},
	"const":      {},
	"export":     {},
	"import":     {},
	"implements": {},
	"let":        {},
	"private":    {},
	"public":     {},
	"yield":      {},
	"interface":  {},
	"package":    {},
	"protected":  {},
	"static":     {},
}

func isReservedIdentifier(name string) bool {
	_, ok := reservedIdentifiers[name]
	return ok
}

var isValidJSNameRegexp = regexp.MustCompile(`(?m)^[\pL_][\pL\pN_]*$`)

func isValidJSName(name string) bool {
	return isReservedIdentifier(name) || isValidJSNameRegexp.MatchString(name)
}

func hasOption(opt string, options []string) bool {
	for _, o := range options {
		if o == opt {
			return true
		}
	}

	return false
}
