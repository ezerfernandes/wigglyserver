package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	lua "github.com/Shopify/go-lua"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

type Page struct {
	Title string
	Body  []byte
}

func (p *Page) save() error {
	filename := Env.PagesDirectory + string(os.PathSeparator) + p.Title + ".md"
	return os.WriteFile(filename, p.Body, 0600)
}

func loadPage(title string) (*Page, error) {
	filename := Env.PagesDirectory + string(os.PathSeparator) + title + ".md"
	body, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return &Page{Title: title, Body: body}, nil
}

func (p *Page) render() []byte {
	// Create new Lua state and open libraries
	l := lua.NewState()
	lua.OpenLibraries(l)

	// Create page module
	l.NewTable() // Create main table
	// l.PushGoFunction(func(l *lua.State) int {
	// 	// Get AST
	// 	extensions := parser.CommonExtensions | parser.AutoHeadingIDs
	// 	p := parser.NewWithExtensions(extensions)
	// 	doc := p.Parse(p.Body)

	// 	// Create AST table
	// 	l.NewTable()
	// 	// Push AST data into table
	// 	pushNode(l, doc)
	// 	return 1
	// })
	// l.SetField(-2, "getAST") // Add getAST function to table

	// Add other page properties
	l.PushString(p.Title)
	l.SetField(-2, "title")

	l.SetGlobal("page") // Set as global 'page'

	// Find all script blocks and execute them
	re := regexp.MustCompile("(?s)```script\n(.*?)```")
	body := string(p.Body)

	// Replace each script block with its output
	body = re.ReplaceAllStringFunc(body, func(match string) string {
		// Extract script content
		script := re.FindStringSubmatch(match)[1]

		// Create string builder to capture output
		var output strings.Builder
		l.Register("print", func(l *lua.State) int {
			// Get the argument passed to print
			str := lua.CheckString(l, 1)
			output.WriteString(str)
			output.WriteString("\n") // Add newline after each print
			return 0
		})

		// Execute the script
		if err := lua.DoString(l, script); err != nil {
			return fmt.Sprintf("Error executing script: %v", err)
		}

		// Trim the trailing newline if exists
		return strings.TrimSuffix(output.String(), "\n")
	})

	return markdownToHTML([]byte(body))
}

func markdownToHTML(md []byte) []byte {
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)

	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	return markdown.Render(doc, renderer)
}

type Environment struct {
	PagesDirectory string
}

func (e *Environment) load() {
	envPagesDirectory := os.Getenv("PAGES_DIR")
	if envPagesDirectory != "" {
		e.PagesDirectory = envPagesDirectory
	}
}

// Default values for global environment
var Env = Environment{
	PagesDirectory: ".",
}

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hi there, I love %s!", r.URL.Path)
}

func pageHandler(w http.ResponseWriter, r *http.Request) {
	title := r.URL.Path[len("/view/"):]
	p, err := loadPage(title)
	if err != nil {
		log.Printf("Page not found: %s", err)
		http.NotFound(w, r)
		return
	}
	fmt.Fprintf(w, "%s", p.render())
}

// Helper function to convert AST nodes to Lua tables
func pushNode(l *lua.State, node ast.Node) {
	l.NewTable()

	// Push node type
	l.PushString(fmt.Sprintf("%T", node))
	l.SetField(-2, "type")

	// Handle different node types
	switch n := node.(type) {
	case *ast.Document:
		l.NewTable()
		for i, child := range n.Children {
			pushNode(l, child)
			l.RawSetInt(-2, i+1)
		}
		l.SetField(-2, "children")

	case *ast.Paragraph:
		l.NewTable()
		for i, child := range n.Children {
			pushNode(l, child)
			l.RawSetInt(-2, i+1)
		}
		l.SetField(-2, "children")

	case *ast.Text:
		l.PushString(string(n.Literal))
		l.SetField(-2, "literal")

		// Add more cases for other node types as needed
	}
}

func main() {
	Env.load()

	http.HandleFunc("/", handler)
	http.HandleFunc("/view/", pageHandler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
