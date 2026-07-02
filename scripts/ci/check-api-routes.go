package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type OpenAPISpec struct {
	Paths map[string]map[string]interface{} `yaml:"paths"`
}

var (
	methodRe      = regexp.MustCompile(`\.(Get|Post|Put|Patch|Delete|Head|Options)\(`)
	routeRe       = regexp.MustCompile(`\.Route\(`)
	groupRe       = regexp.MustCompile(`\.Group\(`)
	chainMethodRe = regexp.MustCompile(`\.With\(.*?\)\s*\.(Get|Post|Put|Patch|Delete|Head|Options)\(`)
)

type route struct {
	method string
	path   string
}

func main() {
	openAPIPath := "docs/api/openapi.yaml"
	codeRoot := "backend/internal"
	if len(os.Args) > 1 {
		openAPIPath = os.Args[1]
	}
	if len(os.Args) > 2 {
		codeRoot = os.Args[2]
	}

	fmt.Fprintf(os.Stderr, "info: checking %s against routes in %s\n", openAPIPath, codeRoot)

	spec := parseOpenAPI(openAPIPath)
	documented := extractRoutes(spec)

	implemented := scanGoRoutes(codeRoot)

	missing := diff(implemented, documented)
	extra := diff(documented, implemented)

	hasErrors := false
	if len(missing) > 0 {
		vals := make([]string, 0, len(missing))
		for k := range missing {
			vals = append(vals, k)
		}
		sort.Strings(vals)
		fmt.Println("❌ Routes in code but NOT in OpenAPI spec:")
		for _, r := range vals {
			fmt.Printf("  %s\n", r)
		}
		hasErrors = true
	}
	if len(extra) > 0 {
		vals := make([]string, 0, len(extra))
		for k := range extra {
			vals = append(vals, k)
		}
		sort.Strings(vals)
		fmt.Println("⚠️  Routes in OpenAPI spec but NOT in code:")
		for _, r := range vals {
			fmt.Printf("  %s\n", r)
		}
	}

	if hasErrors {
		os.Exit(1)
	}
	fmt.Println("✅ All routes match between code and OpenAPI spec")
}

func parseOpenAPI(path string) OpenAPISpec {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: reading OpenAPI spec %s: %v\n", path, err)
		os.Exit(1)
	}
	var spec OpenAPISpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: parsing OpenAPI spec %s: %v\n", path, err)
		os.Exit(1)
	}
	return spec
}

func extractRoutes(spec OpenAPISpec) map[string]bool {
	routes := make(map[string]bool)
	for path, methods := range spec.Paths {
		for method := range methods {
			if method == "parameters" {
				continue
			}
			routes[formatRoute(strings.ToUpper(method), path)] = true
		}
	}
	return routes
}

func scanGoRoutes(root string) map[string]bool {
	routes := make(map[string]bool)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		scanFile(path, routes)
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: scanning Go files in %s: %v\n", root, err)
		os.Exit(1)
	}
	return routes
}

func scanFile(path string, routes map[string]bool) {
	if strings.HasSuffix(path, "_test.go") {
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	content := string(data)
	lines := strings.Split(content, "\n")

	var prefixes []string
	var routeDepth int

	// Only match chi router patterns: lowercase short variable like r.Get(, router.Get(, mux.Get(
	chiMethodRe := regexp.MustCompile(`\b[a-z]{1,4}\.(Get|Post|Put|Patch|Delete|Head|Options)\(`)
	chiRouteRe := regexp.MustCompile(`\b[a-z]{1,4}\.Route\(`)
	chiGroupRe := regexp.MustCompile(`\b[a-z]{1,4}\.Group\(`)

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		openParen := strings.Count(trimmed, "{")
		closeParen := strings.Count(trimmed, "}")

		if routeDepth > 0 {
			routeDepth += openParen
			routeDepth -= closeParen
			if routeDepth <= 0 {
				prefixes = prefixes[:len(prefixes)-1]
				routeDepth = 0
			}
		}

		if chiRouteRe.MatchString(trimmed) || chiGroupRe.MatchString(trimmed) {
			prefix := extractStringArg(trimmed)
			if prefix != "" {
				prefixes = append(prefixes, prefix)
				routeDepth = strings.Count(trimmed, "{") - strings.Count(trimmed, "}")
				if routeDepth <= 0 && strings.Count(trimmed, "func(") == 0 {
					routeDepth = 1
				}
			}
		}

		method, routePath := extractRouteInternal(chiMethodRe, trimmed)
		if method != "" && routePath != "" {
			if routePath == "/*" {
				continue
			}
			fullPath := joinPrefixes(prefixes) + routePath
			if !isInternalRoute(method, fullPath) {
				routes[formatRoute(method, fullPath)] = true
			}
		}
	}
}

func extractStringArg(line string) string {
	idx := strings.Index(line, `"`)
	if idx == -1 {
		return ""
	}
	end := strings.Index(line[idx+1:], `"`)
	if end == -1 {
		return ""
	}
	return line[idx+1 : idx+1+end]
}

func extractRoute(line string) (string, string) {
	if chainMethodRe.MatchString(line) {
		m := chainMethodRe.FindStringSubmatch(line)
		return m[1], extractStringArg(line)
	}
	if methodRe.MatchString(line) {
		m := methodRe.FindStringSubmatch(line)
		return m[1], extractStringArg(line)
	}
	return "", ""
}

func extractRouteInternal(re *regexp.Regexp, line string) (string, string) {
	if re.MatchString(line) {
		m := re.FindStringSubmatch(line)
		return m[1], extractStringArg(line)
	}
	return "", ""
}

func joinPrefixes(prefixes []string) string {
	return strings.Join(prefixes, "")
}

func isInternalRoute(method, path string) bool {
	// Skip Prometheus metrics and debug endpoints
	if strings.HasPrefix(path, "/metrics") {
		return true
	}
	if strings.HasPrefix(path, "/health") {
		return true
	}
	if path == "/" || path == "/debug" || path == "/favicon.ico" {
		return true
	}
	// Skip WebSocket upgrade path (handled externally)
	if strings.Contains(path, "/ws") {
		return true
	}
	return false
}

func formatRoute(method, path string) string {
	return method + " " + path
}

func diff(a, b map[string]bool) map[string]bool {
	result := make(map[string]bool)
	for k := range a {
		if !b[k] {
			result[k] = true
		}
	}
	return result
}
