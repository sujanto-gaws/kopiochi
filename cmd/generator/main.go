package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"
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
	fields := flag.String("fields", "name:string,description:string", "Fields as name:type pairs (e.g., name:string,price:float64)")
	module := flag.String("module", "github.com/sujanto-gaws/kopiochi", "Go module path")
	output := flag.String("output", "internal", "Output directory")
	author := flag.String("author", "", "Author name")
	table := flag.String("table", "", "Database table name (default: pluralized domain)")
	flag.Parse()

	if *domain == "" {
		fmt.Println("Error: -domain is required")
		fmt.Println("Usage: go run cmd/generator/main.go -domain Product -fields \"name:string,price:float64,stock:int\"")
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

	// Parse fields
	config.Fields = parseFields(*fields)

	// Generate table name if not provided
	if config.TableName == "" {
		config.TableName = pluralize(toSnakeCase(config.Domain))
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

	baseDir := config.OutputDir

	for path, tmplContent := range templates {
		filePath := filepath.Join(baseDir, config.DomainLower, path)

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

	return nil
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
{{range .Fields}}{{if .Required}}	if strings.TrimSpace(e.{{.Name}}) == "" {
		return ErrInvalid{{ $domain }}
	}
{{end}}{{end}}	return nil
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

	"github.com/sujanto-gaws/kopiochi/internal/domain/{{.DomainLower}}"
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

	"github.com/sujanto-gaws/kopiochi/internal/domain/{{.DomainLower}}"
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
	dbModel := toDBModel(entity)
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
	var dbModel {{.DomainLower}}DBModel
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
	return toEntity(&dbModel), nil
}

// GetAll retrieves all {{.DomainLower}}s with pagination
func (r *{{.DomainLower}}Repository) GetAll(ctx context.Context, page, limit int) ([]*{{.DomainLower}}.{{.Domain}}, int64, error) {
	var dbModels []{{.DomainLower}}DBModel

	count, err := r.db.NewSelect().Model((*{{.DomainLower}}DBModel)(nil)).Count(ctx)
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
		entities[i] = toEntity(&dbModels[i])
	}

	return entities, int64(count), nil
}

// Update updates an existing {{.DomainLower}}
func (r *{{.DomainLower}}Repository) Update(ctx context.Context, entity *{{.DomainLower}}.{{.Domain}}) error {
	dbModel := toDBModel(entity)
	_, err := r.db.NewUpdate().Model(dbModel).WherePK().Exec(ctx)
	if err == nil {
		entity.UpdatedAt = dbModel.UpdatedAt
	}
	return err
}

// Delete removes a {{.DomainLower}} by ID
func (r *{{.DomainLower}}Repository) Delete(ctx context.Context, id {{.PrimaryKey.Type}}) error {
	_, err := r.db.NewDelete().Model((*{{.DomainLower}}DBModel)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

// toEntity converts database model to domain entity
func toEntity(dbModel *{{.DomainLower}}DBModel) *{{.DomainLower}}.{{.Domain}} {
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
func toDBModel(entity *{{.DomainLower}}.{{.Domain}}) *{{.DomainLower}}DBModel {
	if entity == nil {
		return nil
	}
	return &{{.DomainLower}}DBModel{
		ID:        entity.ID,
{{range .Fields}}		{{.Name}}:     entity.{{.Name}},
{{end}}		CreatedAt: entity.CreatedAt,
		UpdatedAt: entity.UpdatedAt,
	}
}
`

const infrastructureModelTemplate = `package repository

import (
	"time"

	"github.com/uptrace/bun"
)

// {{.DomainLower}}DBModel is the database model for the {{.TableName}} table
type {{.DomainLower}}DBModel struct {
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

	app{{.DomainLower}} "github.com/sujanto-gaws/kopiochi/internal/application/{{.DomainLower}}"
	"github.com/sujanto-gaws/kopiochi/internal/domain/{{.DomainLower}}"
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

// errorResponse creates a standardized error JSON response
func errorResponse(message string) map[string]string {
	return map[string]string{"error": message}
}

// writeJSON is a helper to write JSON responses
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}
`
