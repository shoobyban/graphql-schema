package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"go/format"

	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"
	"github.com/graphql-go/graphql/language/source"
)

// List of var blocks in generated code
var varlist = []string{}

// Waitlist for variable definitions
var blocklist = map[string]chan struct{}{}

// Variable definitions (map orig to go var name)
var defs = map[string]string{}

// Mutex for concurrent writes
var mutex = &sync.Mutex{}

var fields = []string{}

func getGoName(orig string) string {
	if orig == "String" || orig == "ID" || orig == "Int" || orig == "Float" || orig == "Boolean" {
		return "graphql." + orig
	}
	mutex.Lock()
	if v, ok := defs[orig]; ok {
		mutex.Unlock()
		return v
	}
	if _, ok := blocklist[orig]; !ok {
		blocklist[orig] = make(chan struct{})
	}
	mutex.Unlock()
	select {
	case <-blocklist[orig]:
		mutex.Lock()
		if val, ok := defs[orig]; ok {
			mutex.Unlock()
			return val
		}
		mutex.Unlock()
	}
	panic("can't happen: no blocklist but can't find " + orig)
}

func setGoName(orig, name string) {
	mutex.Lock()
	defs[orig] = name
	mutex.Unlock()
	if _, ok := blocklist[orig]; ok {
		close(blocklist[orig])
	}
}

func getGoType(in ast.Type) string {
	t := in.String()
	switch t {
	case "NonNull":
		n := in.(*ast.NonNull)
		t = "graphql.NewNonNull(" + getGoType(n.Type) + ")"
	case "Named":
		n := in.(*ast.Named)
		t = getGoName(n.Name.Value)
	case "List":
		n := in.(*ast.List)
		t = "graphql.NewList(" + getGoType(n.Type) + ")"
	default:
		panic("unhandled type " + t)
	}
	return t
}

func processInput(u *ast.InputObjectDefinition) {
	varname := strings.ToLower(u.Name.Value) + "Input"
	out := "var " + varname + " = graphql.NewInputObject(graphql.InputObjectConfig{\n\tName: \"" + u.Name.Value + "\",\n"
	setGoName(u.Name.Value, varname)
	if len(u.Fields) > 0 {
		out += "\tFields: graphql.InputObjectConfigFieldMap{\n"
		for _, f := range u.Fields {
			out += fmt.Sprintf("\t\t\"%s\": &graphql.InputObjectFieldConfig{\n", f.Name.Value)
			out += "\tType: " + getGoType(f.Type) + ",\n"
			out += "},\n"
		}
		out += "\t},\n"
	}
	mutex.Lock()
	varlist = append(varlist, out+"})\n")
	mutex.Unlock()
}

func processUnion(u *ast.UnionDefinition) {
	varname := strings.ToLower(u.Name.Value) + "Union"
	out := "var " + varname + " = graphql.NewUnion(graphql.UnionConfig{\n\tName: \"" + u.Name.Value + "\",\n"
	out += "\tTypes: []*graphql.Object{\n"
	setGoName(u.Name.Value, varname)
	for _, f := range u.Types {
		out += "\t\t" + getGoType(f) + ",\n"
	}
	mutex.Lock()
	varlist = append(varlist, out+"}})\n")
	mutex.Unlock()
}

func processInterface(in *ast.InterfaceDefinition) {
	varname := strings.ToLower(in.Name.Value) + "Interface"
	out := "var " + varname + " = graphql.NewInterface(graphql.InterfaceConfig{\n\tName: \"" + in.Name.Value + "\",\n"
	out += "\tFields: graphql.Fields{\n"
	setGoName(in.Name.Value, varname)
	for _, f := range in.Fields {
		addField(varname, f, false)
	}
	mutex.Lock()
	varlist = append(varlist, out+"}})\n")
	mutex.Unlock()
}

func processEnum(n *ast.EnumDefinition) {
	varname := strings.ToLower(n.Name.Value) + "Type"
	out := "var " + varname + " = graphql.NewEnum(graphql.EnumConfig{\n\tName: \"" + n.Name.Value + "\",\n"
	out += "\tValues: graphql.EnumValueConfigMap{\n"
	setGoName(n.Name.Value, varname)
	for i, e := range n.Values {
		out += "\t\"" + e.Name.Value + "\": &graphql.EnumValueConfig{\n\t\tValue: " + strconv.Itoa(i) + ",\n\t},\n"
	}
	mutex.Lock()
	varlist = append(varlist, out+"}})\n")
	mutex.Unlock()
}

func processSchema(n *ast.SchemaDefinition) {
	out := "var schema = graphql.SchemaConfig{\n"
	for _, ot := range n.OperationTypes {
		if ot.Operation == "query" {
			out += "  Query: " + getGoName(ot.Type.Name.Value) + ",\n"
		} else if ot.Operation == "mutation" {
			out += "  Mutation: " + getGoName(ot.Type.Name.Value) + ",\n"
		}
	}
	mutex.Lock()
	varlist = append(varlist, out+"}\n")
	mutex.Unlock()
}

func processObject(n *ast.ObjectDefinition) {
	varname := strings.ToLower(n.Name.Value) + "Object"
	realname := n.Name.Value
	resolve := false
	if n.Name.Value == "Query" {
		varname = "rootQuery"
		realname = "RootQuery"
		resolve = true
	} else if n.Name.Value == "Mutation" {
		varname = "rootMutation"
		realname = "RootMutation"
		resolve = true
	}
	out := "var " + varname + " = graphql.NewObject(graphql.ObjectConfig{\n\tName: \"" + realname + "\",\n\tFields: graphql.Fields{\n"
	setGoName(n.Name.Value, varname)
	for _, f := range n.Fields {
		addField(varname, f, resolve)
	}
	mutex.Lock()
	varlist = append(varlist, out+"}})\n")
	mutex.Unlock()
}

func addField(varname string, f *ast.FieldDefinition, resolve bool) {
	out := fmt.Sprintf("\t%s.AddFieldConfig(\"%s\", &graphql.Field{\n", varname, f.Name.Value)
	out += "\tType: " + getGoType(f.Type) + ",\n"
	if len(f.Arguments) > 0 {
		out += "Args: graphql.FieldConfigArgument{\n"
		for _, a := range f.Arguments {
			t := getGoType(a.Type)
			out += "\t\"" + a.Name.Value + "\": &graphql.ArgumentConfig{\n\t\tType: " + t + ",\n},\n"
		}
		out += "},\n"
	}
	if resolve {
		out += "Resolve: resolves[\"" + f.Name.Value + "\"],\n"
	}
	out += "})\n"
	mutex.Lock()
	fields = append(fields, out)
	mutex.Unlock()
}

func processAST(doc *ast.Document) {
	var wg sync.WaitGroup

	for _, child := range doc.Definitions {
		switch child.GetKind() {
		case "SchemaDefinition":
			wg.Add(1)
			go func(child ast.Node) {
				processSchema(child.(*ast.SchemaDefinition))
				wg.Done()
			}(child)
		case "ObjectDefinition":
			wg.Add(1)
			go func(child ast.Node) {
				processObject(child.(*ast.ObjectDefinition))
				wg.Done()
			}(child)
		case "EnumDefinition":
			wg.Add(1)
			go func(child ast.Node) {
				processEnum(child.(*ast.EnumDefinition))
				wg.Done()
			}(child)
		case "InterfaceDefinition":
			wg.Add(1)
			go func(child ast.Node) {
				processInterface(child.(*ast.InterfaceDefinition))
				wg.Done()
			}(child)
		case "UnionDefinition":
			wg.Add(1)
			go func(child ast.Node) {
				processUnion(child.(*ast.UnionDefinition))
				wg.Done()
			}(child)
		case "InputObjectDefinition":
			wg.Add(1)
			go func(child ast.Node) {
				processInput(child.(*ast.InputObjectDefinition))
				wg.Done()
			}(child)
		default:
			panic("unhandled " + child.GetKind())
		}
	}
	wg.Wait()
}

func main() {
	if len(os.Args) < 2 {
		log.Printf("Usage: ./gen {foo.graphql}")
		os.Exit(-1)
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatalf("Can't open file %s", os.Args[1])
	}
	defer f.Close()
	byteValue, _ := ioutil.ReadAll(f)
	source := source.NewSource(&source.Source{
		Body: byteValue,
		Name: "GraphQL Schema",
	})
	ast, err := parser.Parse(parser.ParseParams{Source: source})
	if err != nil {
		log.Fatalf("failed to parse schema file, error: %v", err)
	}
	processAST(ast)
	out := "package dummy\n import (\"github.com/graphql-go/graphql\")\n func getSchema(resolves map[string]graphql.FieldResolveFn) graphql.SchemaConfig {\n"
	for _, item := range varlist {
		out += item
	}
	for _, item := range fields {
		out += item
	}
	out += "\n return schema\n}"
	b, err := format.Source([]byte(out))
	if err != nil {
		fmt.Println(err)
		fmt.Println(out)
	} else {
		fmt.Println(string(b))
	}
}
