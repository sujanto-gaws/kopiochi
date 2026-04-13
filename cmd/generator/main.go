package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"unicode"

	_ "github.com/jackc/pgx/v5/stdlib"
	"go.yaml.in/yaml/v3"
)

// Config holds the generation configuration
type Config struct {
	Domain      string
	DomainLower string
	Fields      []Field
	ModulePath  string
	OutputDir   string
	Author      string
	TableName   string
	IncludeCRUD bool
}

// Field represents a domain field
type Field struct {
	Name     string
	Type     string
	JSONTag  string
	DBTag    string
	Required bool
}

// TemplateData holds all data for template rendering
type TemplateData struct {
	ModulePath  string
	Domain      string
	DomainLower string
	TableName   string
	Fields      []Field
	PrimaryKey  Field
	CreatedAt   bool
	UpdatedAt   bool
}

func main() {
	domain := flag.String("domain", "", "Domain name (e.g., Product, Order)")
	fields := flag.String("fields", "", "Fields as name:type pairs (e.g., name:string,price:float64). Optional if -table is provided")
	module := flag.String("module", "github.com/sujanto-gaws/kopiochi", "Go module path")
	output := flag.String("output", "internal", "Output directory")
	author := flag.String("author", "", "Author name")
	table := flag.String("table", "", "Database table name to read schema from (optional)")
	configFile := flag.String("config", "config/default.yaml", "Path to config file for DB connection")

	flag.Parse()

	if *domain == "" {
		fmt.Println("Error: -domain is required")
		fmt.Println("Usage:")
		fmt.Println("  With explicit fields:")
		fmt.Println("    go run cmd/generator/main.go -domain Product -fields \"name:string,price:float64,stock:int\"")
		fmt.Println("  From existing table schema:")
		fmt.Println("    go run cmd/generator/main.go -domain Product -table products")
		os.Exit(1)
	}

	config := Config{
		Domain:      *domain,
		DomainLower: strings.ToLower(*domain),
		ModulePath:  *module,
		OutputDir:   *output,
		Author:      *author,
		TableName:   *table,
		IncludeCRUD: true,
	}

	// Generate table name if not provided
	if config.TableName == "" {
		config.TableName = toSnakeCase(config.Domain)
	}

	// Parse fields or read from database
	if *fields != "" {
		config.Fields = parseFields(*fields)
	} else if *table != "" {
		// Read DB config
		dbConfig, err := loadDBConfig(*configFile)
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		// Read fields from database schema
		dbFields, err := readTableSchema(dbConfig.Host, dbConfig.Port, dbConfig.User, dbConfig.Password, dbConfig.Name, config.TableName)
		if err != nil {
			fmt.Printf("Error reading table schema: %v\n", err)
			fmt.Println("You can either fix the database connection or provide fields manually with -fields")
			os.Exit(1)
		}
		config.Fields = dbFields
		fmt.Printf("✓ Read %d columns from table '%s'\n", len(dbFields), config.TableName)
	} else {
		fmt.Println("Error: either -fields or -table must be provided")
		fmt.Println("Usage:")
		fmt.Println("  With explicit fields: -domain Product -fields \"name:string,price:float64\"")
		fmt.Println("  From existing table:  -domain Product -table products")
		os.Exit(1)
	}

	if err := generate(config); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Generated CRUD for domain: %s\n", config.Domain)
	fmt.Printf("✓ Output directory: %s\n", config.OutputDir)
	fmt.Printf("✓ Files generated: 7\n")
}

func parseFields(fieldsStr string) []Field {
	var fields []Field
	pairs := strings.Split(fieldsStr, ",")

	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(parts) != 2 {
			continue
		}

		name := strings.TrimSpace(parts[0])
		fieldType := strings.TrimSpace(parts[1])

		fields = append(fields, Field{
			Name:     capitalize(name),
			Type:     mapToGoType(fieldType),
			JSONTag:  name,
			DBTag:    toSnakeCase(name),
			Required: true,
		})
	}

	return fields
}

func mapToGoType(dbType string) string {
	typeMap := map[string]string{
		"string":   "string",
		"text":     "string",
		"varchar":  "string",
		"int":      "int64",
		"integer":  "int64",
		"float":    "float64",
		"decimal":  "float64",
		"bool":     "bool",
		"boolean":  "bool",
		"time":     "time.Time",
		"datetime": "time.Time",
		"uuid":     "string",
	}

	if goType, ok := typeMap[strings.ToLower(dbType)]; ok {
		return goType
	}
	return "string"
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return string(unicode.ToUpper(rune(s[0]))) + s[1:]
}

func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			result.WriteRune('_')
		}
		result.WriteRune(unicode.ToLower(r))
	}
	return result.String()
}

func pluralize(s string) string {
	if strings.HasSuffix(s, "y") {
		return s[:len(s)-1] + "ies"
	}
	return s + "s"
}

func generate(config Config) error {
	data := TemplateData{
		ModulePath:  config.ModulePath,
		Domain:      config.Domain,
		DomainLower: strings.ToLower(config.Domain),
		TableName:   config.TableName,
		Fields:      config.Fields,
		CreatedAt:   true,
		UpdatedAt:   true,
	}

	// Determine primary key (default: ID int64)
	data.PrimaryKey = Field{
		Name:  "ID",
		Type:  "int64",
		DBTag: "id",
	}

	templates := map[string]string{
		"domain/entity.go":             domainEntityTemplate,
		"domain/repository.go":         domainRepositoryTemplate,
		"domain/dto.go":                domainDTOTemplate,
		"application/service.go":       applicationServiceTemplate,
		"infrastructure/repository.go": infrastructureRepositoryTemplate,
		"infrastructure/model.go":      infrastructureModelTemplate,
		"infrastructure/handler.go":    infrastructureHandlerTemplate,
	}

	// Map template paths to correct DDD structure:
	// domain/entity.go → internal/domain/{domain}/entity.go
	// application/service.go → internal/application/{domain}/service.go
	// infrastructure/repository.go → internal/infrastructure/persistence/repository/{domain}.go
	// infrastructure/model.go → internal/infrastructure/persistence/repository/{domain}_model.go
	// infrastructure/handler.go → internal/infrastructure/http/handlers/{domain}_handler.go
	baseDir := config.OutputDir
	pathMapper := map[string]string{
		"domain/entity.go":             filepath.Join(baseDir, "domain", config.DomainLower, "entity.go"),
		"domain/repository.go":         filepath.Join(baseDir, "domain", config.DomainLower, "repository.go"),
		"domain/dto.go":                filepath.Join(baseDir, "domain", config.DomainLower, "dto.go"),
		"application/service.go":       filepath.Join(baseDir, "application", config.DomainLower, "service.go"),
		"infrastructure/repository.go": filepath.Join(baseDir, "infrastructure", "persistence", "repository", config.DomainLower+"_repository.go"),
		"infrastructure/model.go":      filepath.Join(baseDir, "infrastructure", "persistence", "models", config.DomainLower+"_model.go"),
		"infrastructure/handler.go":    filepath.Join(baseDir, "infrastructure", "http", "handlers", config.DomainLower+"_handler.go"),
	}

	for path, tmplContent := range templates {
		filePath := pathMapper[path]

		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", filepath.Dir(filePath), err)
		}

		tmpl, err := template.New(filepath.Base(path)).Parse(tmplContent)
		if err != nil {
			return fmt.Errorf("parse template %s: %w", path, err)
		}

		file, err := os.Create(filePath)
		if err != nil {
			return fmt.Errorf("create file %s: %w", filePath, err)
		}

		if err := tmpl.Execute(file, data); err != nil {
			file.Close()
			return fmt.Errorf("execute template %s: %w", path, err)
		}

		file.Close()
		fmt.Printf("  ✓ %s\n", filePath)
	}

	// Auto-update routes
	if err := updateRoutes(baseDir, config.Domain, config.DomainLower); err != nil {
		fmt.Printf("  ⚠ Warning: Could not update routes: %v\n", err)
		fmt.Printf("  You may need to manually add routes to internal/infrastructure/http/routes/routes.go\n")
	} else {
		fmt.Printf("  ✓ Routes updated in internal/infrastructure/http/routes/routes.go\n")
	}

	// Auto-update main.go for dependency injection
	if err := updateMainGo(baseDir, config.Domain, config.DomainLower, config.ModulePath); err != nil {
		fmt.Printf("  ⚠ Warning: Could not update main.go: %v\n", err)
		fmt.Printf("  You may need to manually wire the handler/service in cmd/api/main.go\n")
	} else {
		fmt.Printf("  ✓ Dependency injection updated in cmd/api/main.go\n")
	}

	return nil
}

// updateRoutes adds new routes to the routes.go file
func updateRoutes(baseDir, domain, domainLower string) error {
	routesFile := filepath.Join(baseDir, "infrastructure", "http", "routes", "routes.go")

	content, err := os.ReadFile(routesFile)
	if err != nil {
		return fmt.Errorf("read routes file: %w", err)
	}

	routesStr := string(content)

	// Check if handler parameter already exists in Setup signature
	if contains(routesStr, fmt.Sprintf("*handlers.%sHandler", domain)) {
		return nil // Already added
	}

	// Add handler parameter to Setup function
	oldSetup := findSetupSignature(routesStr)
	if oldSetup == "" {
		return fmt.Errorf("could not find Setup function signature")
	}

	newSetup := oldSetup[:len(oldSetup)-1] + fmt.Sprintf(", %sHandler *handlers.%sHandler)", domainLower, domain)
	routesStr = strings.Replace(routesStr, oldSetup, newSetup, 1)

	// Add routes for the new domain
	routeBlock := fmt.Sprintf(`
		// %s routes
		r.Post("/%ss", %sHandler.Create%s())
		r.Get("/%ss", %sHandler.GetAll%s())
		r.Get("/%ss/{id}", %sHandler.Get%s())
		r.Put("/%ss/{id}", %sHandler.Update%s())
		r.Delete("/%ss/{id}", %sHandler.Delete%s())`,
		domain, domainLower, domainLower, domain,
		domainLower, domainLower, domain+"s",
		domainLower, domainLower, domain,
		domainLower, domainLower, domain,
		domainLower, domainLower, domain,
	)

	// Find the last route block and add after it
	lastRouteEnd := strings.LastIndex(routesStr, "})")
	if lastRouteEnd == -1 {
		return fmt.Errorf("could not find route block end")
	}

	routesStr = routesStr[:lastRouteEnd] + routeBlock + "\n\t\t" + routesStr[lastRouteEnd:]

	return os.WriteFile(routesFile, []byte(routesStr), 0644)
}

func findSetupSignature(content string) string {
	start := strings.Index(content, "func Setup(")
	if start == -1 {
		return ""
	}

	// Find the closing parenthesis
	parenCount := 0
	for i := start; i < len(content); i++ {
		if content[i] == '(' {
			parenCount++
		} else if content[i] == ')' {
			parenCount--
			if parenCount == 0 {
				return content[start : i+1]
			}
		}
	}

	return ""
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// dbConfig holds database connection settings from config file
type dbConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
}

// loadDBConfig reads database configuration from YAML file
func loadDBConfig(configPath string) (*dbConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg struct {
		DB struct {
			Host     string `yaml:"host"`
			Port     int    `yaml:"port"`
			User     string `yaml:"user"`
			Password string `yaml:"password"`
			Name     string `yaml:"name"`
		} `yaml:"db"`
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &dbConfig{
		Host:     cfg.DB.Host,
		Port:     cfg.DB.Port,
		User:     cfg.DB.User,
		Password: cfg.DB.Password,
		Name:     cfg.DB.Name,
	}, nil
}

// readTableSchema reads column information from PostgreSQL system tables
func readTableSchema(host string, port int, user, pass, dbName, tableName string) ([]Field, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, pass, dbName)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	query := `
		SELECT column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_name = $1
		ORDER BY ordinal_position
	`

	rows, err := db.Query(query, tableName)
	if err != nil {
		return nil, fmt.Errorf("query columns: %w", err)
	}
	defer rows.Close()

	var fields []Field
	for rows.Next() {
		var colName, dataType, isNullable string
		if err := rows.Scan(&colName, &dataType, &isNullable); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		// Skip internal columns
		if colName == "id" || colName == "created_at" || colName == "updated_at" {
			continue
		}

		fields = append(fields, Field{
			Name:     capitalize(toCamelCase(colName)),
			Type:     mapDBType(dataType),
			JSONTag:  colName,
			DBTag:    colName,
			Required: isNullable == "NO",
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	if len(fields) == 0 {
		return nil, fmt.Errorf("table '%s' not found or has no columns", tableName)
	}

	return fields, nil
}

// toCamelCase converts snake_case to CamelCase
func toCamelCase(s string) string {
	parts := strings.Split(s, "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

// mapDBType maps PostgreSQL data types to Go types
func mapDBType(dbType string) string {
	typeMap := map[string]string{
		"character varying": "string",
		"text":              "string",
		"character":         "string",
		"varchar":           "string",
		"bigint":            "int64",
		"integer":           "int64",
		"smallint":          "int64",
		"numeric":           "float64",
		"real":              "float64",
		"double precision":  "float64",
		"decimal":           "float64",
		"boolean":           "bool",
		"timestamp":         "time.Time",
		"date":              "time.Time",
		"json":              "string",
		"jsonb":             "string",
		"uuid":              "string",
	}

	if goType, ok := typeMap[strings.ToLower(dbType)]; ok {
		return goType
	}
	return "string"
}

// updateMainGo adds dependency injection for the new domain to main.go
func updateMainGo(baseDir, domain, domainLower, modulePath string) error {
	mainFile := filepath.Join("cmd", "api", "main.go")

	content, err := os.ReadFile(mainFile)
	if err != nil {
		return fmt.Errorf("read main.go: %w", err)
	}

	mainStr := string(content)

	// 1. Add import for application service
	appImport := fmt.Sprintf(`app%s "%s/internal/application/%s"`, domain, modulePath, domainLower)
	if !contains(mainStr, appImport) {
		mainStr = addImport(mainStr, appImport)
	}

	// 2. Add repository variable after the last repository instantiation
	repoVar := fmt.Sprintf("%sRepo := repository.New%sRepository(bunDB)", domainLower, domain)
	if !contains(mainStr, fmt.Sprintf("%sRepo", domainLower)) {
		repoPattern := regexp.MustCompile(`(\w+Repo := repository\.New\w+Repository\(bunDB\))`)
		if matches := repoPattern.FindAllStringSubmatchIndex(mainStr, -1); len(matches) > 0 {
			lastMatch := matches[len(matches)-1]
			mainStr = mainStr[:lastMatch[1]] + "\n\t\t\t" + repoVar + mainStr[lastMatch[1]:]
		}
	}

	// 3. Add service variable after the last service instantiation
	svcVar := fmt.Sprintf("%sSvc := app%s.NewService(%sRepo)", domainLower, domain, domainLower)
	if !contains(mainStr, fmt.Sprintf("%sSvc", domainLower)) {
		svcPattern := regexp.MustCompile(`(\w+Svc := app\w+\.NewService\(\w+Repo\))`)
		if matches := svcPattern.FindAllStringSubmatchIndex(mainStr, -1); len(matches) > 0 {
			lastMatch := matches[len(matches)-1]
			mainStr = mainStr[:lastMatch[1]] + "\n\t\t\t" + svcVar + mainStr[lastMatch[1]:]
		}
	}

	// 4. Add handler variable after the last handler instantiation
	handlerVar := fmt.Sprintf("%sHandler := handlers.New%sHandler(%sSvc)", domainLower, domain, domainLower)
	if !contains(mainStr, fmt.Sprintf("%sHandler := handlers.New", domainLower)) {
		handlerPattern := regexp.MustCompile(`(\w+Handler := handlers\.New\w+Handler\(\w+Svc\))`)
		if matches := handlerPattern.FindAllStringSubmatchIndex(mainStr, -1); len(matches) > 0 {
			lastMatch := matches[len(matches)-1]
			mainStr = mainStr[:lastMatch[1]] + "\n\t\t\t" + handlerVar + mainStr[lastMatch[1]:]
		}
	}

	// 5. Update routes.Setup call to include new handler
	setupPattern := regexp.MustCompile(`(routes\.Setup\(r,[^)]+)\)`)
	if setupPattern.MatchString(mainStr) && !contains(mainStr, fmt.Sprintf("%sHandler)", domainLower)) {
		mainStr = setupPattern.ReplaceAllString(mainStr, fmt.Sprintf("${1}, %sHandler)", domainLower))
	}

	return os.WriteFile(mainFile, []byte(mainStr), 0644)
}

func addImport(content, newImport string) string {
	// Find the last import line and add after it
	lines := strings.Split(content, "\n")
	lastImportIdx := -1

	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), `"`) && strings.HasSuffix(strings.TrimSpace(line), `"`) {
			lastImportIdx = i
		}
	}

	if lastImportIdx > 0 {
		lines = append(lines[:lastImportIdx+1], append([]string{newImport}, lines[lastImportIdx+1:]...)...)
	}

	return strings.Join(lines, "\n")
}

type matchResult struct {
	str string
	end int
}

func findLastMatch(content, pattern string) matchResult {
	// Simple pattern matching (not regex, just string search for common patterns)
	// This is a simplified version - for production use, use regexp package
	idx := strings.LastIndex(content, pattern)
	if idx == -1 {
		return matchResult{"", -1}
	}

	// Find the end of the line
	endIdx := idx + len(pattern)
	for endIdx < len(content) && content[endIdx] != '\n' {
		endIdx++
	}

	return matchResult{content[idx:endIdx], endIdx}
}

// ==================== TEMPLATES ====================

const domainEntityTemplate = `package {{.DomainLower}}

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalid{{.Domain}} = errors.New("invalid {{.DomainLower}}")
	ErrNotFound           = errors.New("{{.DomainLower}} not found")
)

// {{.Domain}} is the domain entity representing a {{.DomainLower}} in the system
type {{.Domain}} struct {
	ID        {{.PrimaryKey.Type}}
{{range .Fields}}	{{.Name}}     {{.Type}}
{{end}}	CreatedAt time.Time
	UpdatedAt time.Time
}

// Validate validates the {{.DomainLower}} entity against business rules
func (e *{{.Domain}}) Validate() error {
{{- $domain := .Domain }}
{{range .Fields}}{{if .Required}}{{if eq .Type "string"}}	if strings.TrimSpace(e.{{.Name}}) == "" {
		return ErrInvalid{{ $domain }}
	}
{{end}}{{end}}{{end}}	return nil
}
`

const domainRepositoryTemplate = `package {{.DomainLower}}

import (
	"context"
)

// Repository defines the domain interface for {{.DomainLower}} persistence
type Repository interface {
	Create(ctx context.Context, entity *{{.Domain}}) error
	GetByID(ctx context.Context, id {{.PrimaryKey.Type}}) (*{{.Domain}}, error)
	GetAll(ctx context.Context, page, limit int) ([]*{{.Domain}}, int64, error)
	Update(ctx context.Context, entity *{{.Domain}}) error
	Delete(ctx context.Context, id {{.PrimaryKey.Type}}) error
}
`

const domainDTOTemplate = `package {{.DomainLower}}

import (
	"time"
)

// Create{{.Domain}}Request represents the request body for creating a {{.DomainLower}}
type Create{{.Domain}}Request struct {
{{range .Fields}}	{{.Name}} {{.Type}} ` + "`" + `json:"{{.JSONTag}}" validate:"required"` + "`" + `
{{end}}}

// Update{{.Domain}}Request represents the request body for updating a {{.DomainLower}}
type Update{{.Domain}}Request struct {
{{range .Fields}}	{{.Name}} {{.Type}} ` + "`" + `json:"{{.JSONTag}}" validate:"required"` + "`" + `
{{end}}}

// {{.Domain}}Response represents the response body for {{.DomainLower}} operations
type {{.Domain}}Response struct {
	ID        {{.PrimaryKey.Type}} ` + "`" + `json:"id"` + "`" + `
{{range .Fields}}	{{.Name}}     {{.Type}} ` + "`" + `json:"{{.JSONTag}}"` + "`" + `
{{end}}	CreatedAt time.Time ` + "`" + `json:"created_at"` + "`" + `
	UpdatedAt time.Time ` + "`" + `json:"updated_at"` + "`" + `
}

// To{{.Domain}}Response converts a domain entity to DTO
func To{{.Domain}}Response(e *{{.Domain}}) *{{.Domain}}Response {
	if e == nil {
		return nil
	}
	return &{{.Domain}}Response{
		ID:        e.ID,
{{range .Fields}}		{{.Name}}:     e.{{.Name}},
{{end}}		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}

// To{{.Domain}}Responses converts a slice of entities to DTOs
func To{{.Domain}}Responses(entities []*{{.Domain}}) []*{{.Domain}}Response {
	responses := make([]*{{.Domain}}Response, len(entities))
	for i, e := range entities {
		responses[i] = To{{.Domain}}Response(e)
	}
	return responses
}

// ToEntity converts CreateRequest to domain entity
func (r *Create{{.Domain}}Request) ToEntity() *{{.Domain}} {
	return &{{.Domain}}{
{{range .Fields}}		{{.Name}}: r.{{.Name}},
{{end}}	}
}
`

const applicationServiceTemplate = `package {{.DomainLower}}

import (
	"context"

	"{{.ModulePath}}/internal/domain/{{.DomainLower}}"
)

// Service implements the {{.DomainLower}} application service
type Service struct {
	repo {{.DomainLower}}.Repository
}

// NewService creates a new {{.DomainLower}} service
func NewService(repo {{.DomainLower}}.Repository) *Service {
	return &Service{repo: repo}
}

// Create{{.Domain}} creates a new {{.DomainLower}}
func (s *Service) Create{{.Domain}}(ctx context.Context, req *{{.DomainLower}}.Create{{.Domain}}Request) (*{{.DomainLower}}.{{.Domain}}Response, error) {
	entity := req.ToEntity()

	if err := entity.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	return {{.DomainLower}}.To{{.Domain}}Response(entity), nil
}

// Get{{.Domain}}ByID retrieves a {{.DomainLower}} by ID
func (s *Service) Get{{.Domain}}ByID(ctx context.Context, id {{.PrimaryKey.Type}}) (*{{.DomainLower}}.{{.Domain}}Response, error) {
	if id <= 0 {
		return nil, {{.DomainLower}}.ErrNotFound
	}

	entity, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, {{.DomainLower}}.ErrNotFound
	}

	return {{.DomainLower}}.To{{.Domain}}Response(entity), nil
}

// GetAll{{.Domain}}s retrieves all {{.DomainLower}}s with pagination
func (s *Service) GetAll{{.Domain}}s(ctx context.Context, page, limit int) ([]*{{.DomainLower}}.{{.Domain}}Response, int64, error) {
	entities, total, err := s.repo.GetAll(ctx, page, limit)
	if err != nil {
		return nil, 0, err
	}

	return {{.DomainLower}}.To{{.Domain}}Responses(entities), total, nil
}

// Update{{.Domain}} updates an existing {{.DomainLower}}
func (s *Service) Update{{.Domain}}(ctx context.Context, id {{.PrimaryKey.Type}}, req *{{.DomainLower}}.Update{{.Domain}}Request) (*{{.DomainLower}}.{{.Domain}}Response, error) {
	entity, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, {{.DomainLower}}.ErrNotFound
	}
{{range .Fields}}	entity.{{.Name}} = req.{{.Name}}
{{end}}
	if err := entity.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return {{.DomainLower}}.To{{.Domain}}Response(entity), nil
}

// Delete{{.Domain}} deletes a {{.DomainLower}} by ID
func (s *Service) Delete{{.Domain}}(ctx context.Context, id {{.PrimaryKey.Type}}) error {
	entity, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if entity == nil {
		return {{.DomainLower}}.ErrNotFound
	}

	return s.repo.Delete(ctx, id)
}
`

const infrastructureRepositoryTemplate = `package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"{{.ModulePath}}/internal/domain/{{.DomainLower}}"
	"{{.ModulePath}}/internal/infrastructure/persistence/models"
)

// {{.DomainLower}}Repository implements the {{.DomainLower}}.Repository interface
type {{.DomainLower}}Repository struct {
	db bun.IDB
}

// New{{.Domain}}Repository creates a new {{.DomainLower}} repository
func New{{.Domain}}Repository(db bun.IDB) {{.DomainLower}}.Repository {
	return &{{.DomainLower}}Repository{db: db}
}

// Create persists a new {{.DomainLower}}
func (r *{{.DomainLower}}Repository) Create(ctx context.Context, entity *{{.DomainLower}}.{{.Domain}}) error {
	dbModel := to{{.Domain}}DBModel(entity)
	_, err := r.db.NewInsert().Model(dbModel).Exec(ctx)
	if err == nil {
		entity.ID = dbModel.ID
		entity.CreatedAt = dbModel.CreatedAt
		entity.UpdatedAt = dbModel.UpdatedAt
	}
	return err
}

// GetByID retrieves a {{.DomainLower}} by ID
func (r *{{.DomainLower}}Repository) GetByID(ctx context.Context, id {{.PrimaryKey.Type}}) (*{{.DomainLower}}.{{.Domain}}, error) {
	var dbModel models.{{.Domain}}DBModel
	err := r.db.NewSelect().
		Model(&dbModel).
		Where("id = ?", id).
		Scan(ctx)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return to{{.Domain}}Entity(&dbModel), nil
}

// GetAll retrieves all {{.DomainLower}}s with pagination
func (r *{{.DomainLower}}Repository) GetAll(ctx context.Context, page, limit int) ([]*{{.DomainLower}}.{{.Domain}}, int64, error) {
	var dbModels []models.{{.Domain}}DBModel

	count, err := r.db.NewSelect().Model((*models.{{.Domain}}DBModel)(nil)).Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	err = r.db.NewSelect().
		Model(&dbModels).
		Order("id ASC").
		Limit(limit).
		Offset(offset).
		Scan(ctx)

	if err != nil {
		return nil, 0, err
	}

	entities := make([]*{{.DomainLower}}.{{.Domain}}, len(dbModels))
	for i := range dbModels {
		entities[i] = to{{.Domain}}Entity(&dbModels[i])
	}

	return entities, int64(count), nil
}

// Update updates an existing {{.DomainLower}}
func (r *{{.DomainLower}}Repository) Update(ctx context.Context, entity *{{.DomainLower}}.{{.Domain}}) error {
	dbModel := to{{.Domain}}DBModel(entity)
	_, err := r.db.NewUpdate().Model(dbModel).WherePK().Exec(ctx)
	if err == nil {
		entity.UpdatedAt = dbModel.UpdatedAt
	}
	return err
}

// Delete removes a {{.DomainLower}} by ID
func (r *{{.DomainLower}}Repository) Delete(ctx context.Context, id {{.PrimaryKey.Type}}) error {
	_, err := r.db.NewDelete().Model((*models.{{.Domain}}DBModel)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

// to{{.Domain}}Entity converts database model to domain entity
func to{{.Domain}}Entity(dbModel *models.{{.Domain}}DBModel) *{{.DomainLower}}.{{.Domain}} {
	if dbModel == nil {
		return nil
	}
	return &{{.DomainLower}}.{{.Domain}}{
		ID:        dbModel.ID,
{{range .Fields}}		{{.Name}}:     dbModel.{{.Name}},
{{end}}		CreatedAt: dbModel.CreatedAt,
		UpdatedAt: dbModel.UpdatedAt,
	}
}

// toDBModel converts domain entity to database model
func to{{.Domain}}DBModel(entity *{{.DomainLower}}.{{.Domain}}) *models.{{.Domain}}DBModel {
	if entity == nil {
		return nil
	}
	return &models.{{.Domain}}DBModel{
		ID:        entity.ID,
{{range .Fields}}		{{.Name}}:     entity.{{.Name}},
{{end}}		CreatedAt: entity.CreatedAt,
		UpdatedAt: entity.UpdatedAt,
	}
}
`

const infrastructureModelTemplate = `package models

import (
	"time"

	"github.com/uptrace/bun"
)

// {{.Domain}}DBModel is the database model for the {{.TableName}} table
type {{.Domain}}DBModel struct {
	bun.BaseModel ` + "`" + `bun:"table:{{.TableName}},alias:{{.DomainLower}}"` + "`" + `
	ID            {{.PrimaryKey.Type}} ` + "`" + `bun:"id,pk,autoincrement"` + "`" + `
{{range .Fields}}	{{.Name}}     {{.Type}} ` + "`" + `bun:"{{.DBTag}},notnull"` + "`" + `
{{end}}	CreatedAt     time.Time ` + "`" + `bun:"created_at,notnull,default:now()"` + "`" + `
	UpdatedAt     time.Time ` + "`" + `bun:"updated_at,notnull,default:now()"` + "`" + `
}
`

const infrastructureHandlerTemplate = `package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	app{{.DomainLower}} "{{.ModulePath}}/internal/application/{{.DomainLower}}"
	"{{.ModulePath}}/internal/domain/{{.DomainLower}}"
)

// {{.Domain}}Handler handles HTTP requests for {{.DomainLower}} operations
type {{.Domain}}Handler struct {
	svc *app{{.DomainLower}}.Service
}

// New{{.Domain}}Handler creates a new {{.DomainLower}} handler
func New{{.Domain}}Handler(svc *app{{.DomainLower}}.Service) *{{.Domain}}Handler {
	return &{{.Domain}}Handler{svc: svc}
}

// Create{{.Domain}} handles POST /{{.DomainLower}}s
// @Summary Create a new {{.DomainLower}}
// @Description Create a new {{.DomainLower}} with the provided data
// @Tags {{.DomainLower}}s
// @Accept json
// @Produce json
// @Param request body {{.DomainLower}}.Create{{.Domain}}Request true "{{.Domain}} creation request"
// @Success 201 {object} {{.DomainLower}}.{{.Domain}}Response "{{.Domain}} created successfully"
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /{{.DomainLower}}s [post]
func (h *{{.Domain}}Handler) Create{{.Domain}}() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req {{.DomainLower}}.Create{{.Domain}}Request

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid request body"))
			return
		}

		resp, err := h.svc.Create{{.Domain}}(r.Context(), &req)
		if err != nil {
			if err == {{.DomainLower}}.ErrInvalid{{.Domain}} {
				writeJSON(w, http.StatusBadRequest, errorResponse(err.Error()))
				return
			}
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to create {{.DomainLower}}"))
			return
		}

		writeJSON(w, http.StatusCreated, resp)
	}
}

// Get{{.Domain}} handles GET /{{.DomainLower}}s/{id}
// @Summary Get a {{.DomainLower}} by ID
// @Description Retrieve a {{.DomainLower}} by its unique ID
// @Tags {{.DomainLower}}s
// @Produce json
// @Param id path int true "{{.Domain}} ID"
// @Success 200 {object} {{.DomainLower}}.{{.Domain}}Response "{{.Domain}} found"
// @Failure 400 {object} map[string]string "Invalid {{.DomainLower}} ID"
// @Failure 404 {object} map[string]string "{{.DomainLower}} not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /{{.DomainLower}}s/{id} [get]
func (h *{{.Domain}}Handler) Get{{.Domain}}() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid {{.DomainLower}} ID"))
			return
		}

		resp, err := h.svc.Get{{.Domain}}ByID(r.Context(), id)
		if err != nil {
			if err == {{.DomainLower}}.ErrNotFound {
				writeJSON(w, http.StatusNotFound, errorResponse("{{.DomainLower}} not found"))
				return
			}
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to fetch {{.DomainLower}}"))
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// GetAll{{.Domain}}s handles GET /{{.DomainLower}}s
// @Summary Get all {{.DomainLower}}s with pagination
// @Description Retrieve a paginated list of {{.DomainLower}}s
// @Tags {{.DomainLower}}s
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(20)
// @Success 200 {object} map[string]interface{} "List of {{.DomainLower}}s"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /{{.DomainLower}}s [get]
func (h *{{.Domain}}Handler) GetAll{{.Domain}}s() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

		if page <= 0 {
			page = 1
		}
		if limit <= 0 {
			limit = 20
		}

		responses, total, err := h.svc.GetAll{{.Domain}}s(r.Context(), page, limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to fetch {{.DomainLower}}s"))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data":  responses,
			"total": total,
			"page":  page,
			"limit": limit,
		})
	}
}

// Update{{.Domain}} handles PUT /{{.DomainLower}}s/{id}
// @Summary Update an existing {{.DomainLower}}
// @Description Update a {{.DomainLower}} by its ID
// @Tags {{.DomainLower}}s
// @Accept json
// @Produce json
// @Param id path int true "{{.Domain}} ID"
// @Param request body {{.DomainLower}}.Update{{.Domain}}Request true "{{.Domain}} update request"
// @Success 200 {object} {{.DomainLower}}.{{.Domain}}Response "{{.Domain}} updated successfully"
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 404 {object} map[string]string "{{.DomainLower}} not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /{{.DomainLower}}s/{id} [put]
func (h *{{.Domain}}Handler) Update{{.Domain}}() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid {{.DomainLower}} ID"))
			return
		}

		var req {{.DomainLower}}.Update{{.Domain}}Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid request body"))
			return
		}

		resp, err := h.svc.Update{{.Domain}}(r.Context(), id, &req)
		if err != nil {
			switch err {
			case {{.DomainLower}}.ErrNotFound:
				writeJSON(w, http.StatusNotFound, errorResponse("{{.DomainLower}} not found"))
			case {{.DomainLower}}.ErrInvalid{{.Domain}}:
				writeJSON(w, http.StatusBadRequest, errorResponse(err.Error()))
			default:
				writeJSON(w, http.StatusInternalServerError, errorResponse("failed to update {{.DomainLower}}"))
			}
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// Delete{{.Domain}} handles DELETE /{{.DomainLower}}s/{id}
// @Summary Delete a {{.DomainLower}} by ID
// @Description Delete a {{.DomainLower}} by its unique ID
// @Tags {{.DomainLower}}s
// @Param id path int true "{{.Domain}} ID"
// @Success 204 "{{.DomainLower}} deleted successfully"
// @Failure 400 {object} map[string]string "Invalid {{.DomainLower}} ID"
// @Failure 404 {object} map[string]string "{{.DomainLower}} not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /{{.DomainLower}}s/{id} [delete]
func (h *{{.Domain}}Handler) Delete{{.Domain}}() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid {{.DomainLower}} ID"))
			return
		}

		if err := h.svc.Delete{{.Domain}}(r.Context(), id); err != nil {
			if err == {{.DomainLower}}.ErrNotFound {
				writeJSON(w, http.StatusNotFound, errorResponse("{{.DomainLower}} not found"))
				return
			}
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to delete {{.DomainLower}}"))
			return
		}

		writeJSON(w, http.StatusNoContent, nil)
	}
}
`
