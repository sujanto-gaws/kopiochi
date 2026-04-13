# Swagger/OpenAPI Documentation

> **Interactive API Documentation for Kopiochi**

This project uses [Swagger UI](https://swagger.io/tools/swagger-ui/) to provide interactive, auto-generated API documentation. Swagger allows developers and consumers to explore, test, and understand the API without reading source code or making manual requests.

## 📋 Table of Contents

- [Quick Start](#-quick-start)
- [Accessing Swagger UI](#-accessing-swagger-ui)
- [Generating Documentation](#-generating-documentation)
- [Writing Swagger Annotations](#-writing-swagger-annotations)
  - [Basic Structure](#basic-structure)
  - [HTTP Methods](#http-methods)
  - [Parameters](#parameters)
  - [Responses](#responses)
- [Documenting Models](#-documenting-models)
- [Authentication & Security](#-authentication--security)
- [Code Generator Integration](#-code-generator-integration)
- [Best Practices](#-best-practices)
- [Troubleshooting](#-troubleshooting)
- [Make Commands](#-make-commands)

## 🚀 Quick Start

### 1. Generate Swagger Documentation

```bash
make swagger-docs
```

### 2. Start the Server

```bash
make run
```

### 3. Open Swagger UI

Navigate to: **http://localhost:8080/swagger/index.html**

## 🌐 Accessing Swagger UI

Once the server is running, Swagger UI is available at:

| Environment | URL |
|-------------|-----|
| **Local** | http://localhost:8080/swagger/index.html |
| **Production** | https://yourdomain.com/swagger/index.html |

### What You Can Do in Swagger UI

- ✅ **Browse all API endpoints** organized by tags
- ✅ **View request/response schemas** with detailed field descriptions
- ✅ **Test endpoints directly** from the browser
- ✅ **Authenticate with JWT** tokens to test protected endpoints
- ✅ **Download OpenAPI spec** in JSON or YAML format
- ✅ **Export code examples** in multiple languages (cURL, JavaScript, Python, etc.)

## 📝 Generating Documentation

### Initial Generation

```bash
# Generate swagger docs for the first time
make swagger-docs
```

This creates the `docs/` directory with:
- `docs.go` - Go code for embedding swagger
- `swagger.json` - OpenAPI specification in JSON
- `swagger.yaml` - OpenAPI specification in YAML

### Regenerate After Changes

**Important:** Run `make swagger-docs` whenever you:
- Add new endpoints
- Modify existing endpoints
- Change request/response models
- Update swagger annotations

```bash
# Regenerate swagger docs
make swagger-docs

# Rebuild the application
make build
```

### CI/CD Integration

Add swagger generation to your CI/CD pipeline to ensure docs stay in sync:

```yaml
# Example GitHub Actions
- name: Generate Swagger Docs
  run: make swagger-docs

- name: Verify No Changes
  run: git diff --exit-code docs/
```

## ✍️ Writing Swagger Annotations

### Basic Structure

Swagger annotations are special comments placed above handler functions. The format is:

```go
// @Summary Short summary of the endpoint
// @Description Detailed description of what this endpoint does
// @Tags tag-name
// @Accept json
// @Produce json
// @Param name location type required "description"
// @Success code {type} model "description"
// @Failure code {type} model "description"
// @Router /path [method]
func (h *Handler) MyEndpoint() http.HandlerFunc {
    // ...
}
```

### HTTP Methods

#### GET Endpoint

```go
// GetUser handles GET /users/{id}
// @Summary Get a user by ID
// @Description Retrieve a user by their unique ID
// @Tags users
// @Produce json
// @Param id path int true "User ID"
// @Success 200 {object} user.UserResponse "User found"
// @Failure 400 {object} map[string]string "Invalid user ID"
// @Failure 404 {object} map[string]string "User not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /users/{id} [get]
func (h *UserHandler) GetUser() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Implementation
    }
}
```

#### POST Endpoint

```go
// CreateUser handles POST /users
// @Summary Create a new user
// @Description Create a new user with name and email
// @Tags users
// @Accept json
// @Produce json
// @Param request body user.CreateUserRequest true "User creation request"
// @Success 201 {object} user.UserResponse "User created successfully"
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /users [post]
func (h *UserHandler) CreateUser() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Implementation
    }
}
```

#### PUT Endpoint

```go
// UpdateUser handles PUT /users/{id}
// @Summary Update an existing user
// @Description Update a user's name and/or email by their ID
// @Tags users
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Param request body user.UpdateUserRequest true "User update request"
// @Success 200 {object} user.UserResponse "User updated successfully"
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 404 {object} map[string]string "User not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /users/{id} [put]
func (h *UserHandler) UpdateUser() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Implementation
    }
}
```

#### DELETE Endpoint

```go
// DeleteUser handles DELETE /users/{id}
// @Summary Delete a user by ID
// @Description Delete a user by their unique ID
// @Tags users
// @Param id path int true "User ID"
// @Success 204 "User deleted successfully"
// @Failure 400 {object} map[string]string "Invalid user ID"
// @Failure 404 {object} map[string]string "User not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /users/{id} [delete]
func (h *UserHandler) DeleteUser() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Implementation
    }
}
```

### Parameters

#### Path Parameters

```go
// @Param id path int true "User ID"
// @Param username path string true "Username"
// @Param postId path int true "Post ID"
```

#### Query Parameters

```go
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(20)
// @Param search query string false "Search term"
// @Param active query bool false "Filter by active status"
```

#### Request Body

```go
// @Param request body user.CreateUserRequest true "User creation request"
// @Param request body user.UpdateUserRequest true "User update request"
```

#### Header Parameters

```go
// @Param Authorization header string true "Bearer token"
// @Param X-API-Key header string true "API key"
```

### Responses

#### Success Responses

```go
// @Success 200 {object} user.UserResponse "User found"
// @Success 201 {object} user.UserResponse "User created successfully"
// @Success 204 "User deleted successfully"
// @Success 200 {object} map[string]interface{} "List of users"
```

#### Error Responses

```go
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Forbidden"
// @Failure 404 {object} map[string]string "User not found"
// @Failure 500 {object} map[string]string "Internal server error"
```

## 📦 Documenting Models

### DTOs with Swagger Tags

Add descriptions to your DTO fields using comments:

```go
package user

// CreateUserRequest represents the request body for creating a user
type CreateUserRequest struct {
    // User's full name (required)
    // Example: John Doe
    Name string `json:"name" validate:"required,min=2,max=100"`
    
    // User's email address (required)
    // Example: john@example.com
    Email string `json:"email" validate:"required,email"`
}

// UserResponse represents the response body for user operations
type UserResponse struct {
    // Unique identifier
    ID int64 `json:"id"`
    
    // User's full name
    Name string `json:"name"`
    
    // User's email address
    Email string `json:"email"`
    
    // Creation timestamp
    CreatedAt time.Time `json:"created_at"`
    
    // Last update timestamp
    UpdatedAt time.Time `json:"updated_at"`
}
```

### Example Values

Add example values to help API consumers understand the expected format:

```go
type CreateUserRequest struct {
    Name string `json:"name"` // Example: John Doe
    Email string `json:"email"` // Example: john@example.com
}
```

### Nested Models

Swagger automatically resolves nested types. If your response includes nested objects:

```go
type UserResponse struct {
    ID int64 `json:"id"`
    Profile ProfileResponse `json:"profile"` // Nested model
}

type ProfileResponse struct {
    Bio string `json:"bio"`
    Avatar string `json:"avatar"`
}
```

## 🔐 Authentication & Security

### JWT Bearer Authentication

The project includes JWT Bearer authentication configuration. To document protected endpoints:

```go
// @Summary Get user profile
// @Description Retrieve the authenticated user's profile
// @Tags users
// @Produce json
// @Security BearerAuth
// @Success 200 {object} user.UserResponse "User profile"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Router /users/profile [get]
func (h *UserHandler) GetProfile() http.HandlerFunc {
    // Implementation
}
```

### Using BearerAuth in Swagger UI

1. Click the **Authorize** button (lock icon) at the top of Swagger UI
2. Enter your JWT token in the format: `Bearer <your-token>`
3. Click **Authorize**
4. All subsequent requests will include the token

### Multiple Security Schemes

If you have multiple authentication methods:

```go
// In main.go or a dedicated swagger file:

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key
// @description API key for service-to-service authentication
```

### FIDO2/WebAuthn Endpoints

Document FIDO2 authentication flows:

```go
// @Summary Start FIDO2 registration
// @Description Initiate passwordless registration for a user
// @Tags auth
// @Accept json
// @Produce json
// @Param request body auth.FIDO2RegistrationRequest true "FIDO2 registration request"
// @Success 200 {object} auth.FIDO2RegistrationResponse "Registration challenge"
// @Failure 400 {object} map[string]string "Invalid request"
// @Router /auth/fido2/register [post]
```

## 🛠️ Code Generator Integration

### Auto-Generated Swagger

When you generate a new domain using the code generator, Swagger annotations are automatically included:

```bash
make generate DOMAIN=Product FIELDS="name:string,price:float64"
```

This creates handlers with pre-configured swagger annotations:

```go
// CreateProduct handles POST /products
// @Summary Create a new product
// @Description Create a new product with the provided data
// @Tags products
// @Accept json
// @Produce json
// @Param request body product.CreateProductRequest true "Product creation request"
// @Success 201 {object} product.ProductResponse "Product created successfully"
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /products [post]
func (h *ProductHandler) CreateProduct() http.HandlerFunc {
    // ...
}
```

### After Generating a Domain

Remember to regenerate swagger docs:

```bash
# Generate domain
make generate DOMAIN=Product FIELDS="name:string,price:float64"

# Regenerate swagger
make swagger-docs

# Rebuild
make build
```

## 📋 Best Practices

### 1. **Always Document All Endpoints**

Every public endpoint should have complete swagger annotations.

### 2. **Use Descriptive Tags**

Group related endpoints with meaningful tags:

```go
// @Tags users
// @Tags products
// @Tags auth
// @Tags orders
```

### 3. **Provide Clear Descriptions**

- **@Summary**: One-line description (max 60 chars)
- **@Description**: Detailed explanation of endpoint behavior

### 4. **Document All Errors**

Include all possible error responses:

```go
// @Failure 400 {object} map[string]string "Bad Request - Invalid input"
// @Failure 401 {object} map[string]string "Unauthorized - Invalid or missing token"
// @Failure 403 {object} map[string]string "Forbidden - Insufficient permissions"
// @Failure 404 {object} map[string]string "Not Found - Resource does not exist"
// @Failure 409 {object} map[string]string "Conflict - Duplicate resource"
// @Failure 422 {object} map[string]string "Unprocessable Entity - Validation failed"
// @Failure 500 {object} map[string]string "Internal Server Error"
```

### 5. **Include Examples**

Add example values to request/response models to help API consumers.

### 6. **Mark Required Fields**

Clearly indicate required parameters:

```go
// @Param id path int true "User ID"
// @Param request body user.CreateUserRequest true "User creation request"
```

### 7. **Version Your API**

Include API version in BasePath and annotations:

```go
// @BasePath /api/v1
```

### 8. **Keep Swagger in Sync**

**Always regenerate swagger docs after:**
- Adding new endpoints
- Changing endpoint signatures
- Modifying request/response models
- Updating validation rules

### 9. **Use Security Annotations**

Document authentication requirements:

```go
// @Security BearerAuth
```

### 10. **Test Your Documentation**

Regularly test endpoints through Swagger UI to ensure accuracy:

1. Open Swagger UI
2. Click on an endpoint
3. Click "Try it out"
4. Fill in parameters
5. Click "Execute"
6. Verify the response matches expectations

## 🔧 Troubleshooting

### Swagger UI Not Accessible

**Problem:** Can't access http://localhost:8080/swagger/index.html

**Solutions:**
1. Ensure swagger docs are generated: `make swagger-docs`
2. Verify the server is running: `make run`
3. Check routes are configured in `routes.go`
4. Verify `docs` package is imported with blank identifier: `_ "github.com/sujanto-gaws/kopiochi/docs"`

### Build Errors After Swagger Generation

**Problem:** Build fails with errors like `unknown field LeftDelim`

**Solution:**
The swag version may have compatibility issues. Remove the problematic fields from `docs/docs.go`:

```go
var SwaggerInfo = &swag.Spec{
    // ... other fields
    // Remove these lines:
    // LeftDelim:        "{{",
    // RightDelim:       "}}",
}
```

### Missing Types in Swagger UI

**Problem:** Request/response models show as empty or undefined

**Solutions:**
1. Ensure DTOs have proper `json` tags
2. Verify types are imported correctly in handler file
3. Run `make swagger-docs` to regenerate
4. Check for typos in type names in annotations

### Annotations Not Showing

**Problem:** Swagger UI shows endpoints but missing summaries/descriptions

**Solutions:**
1. Verify annotations are directly above the handler function
2. Check for typos in annotation syntax (e.g., `@Summary` not `@summary`)
3. Ensure there's no space between `//` and `@`
4. Run `make swagger-docs` to regenerate

### Wrong Package Name Error

**Problem:** `warning: failed to get package name in dir`

**Solution:**
This is a warning and doesn't affect functionality. You can specify the module explicitly:

```bash
swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal --parseGoPackages ./cmd/api
```

## 🎯 Make Commands

All swagger-related make commands:

| Command | Description |
|---------|-------------|
| `make swagger-init` | Initialize swagger annotations (first-time setup) |
| `make swagger-docs` | Generate swagger documentation from annotations |
| `make swagger-serve` | Display instructions for accessing swagger UI |

### Typical Workflow

```bash
# 1. Make changes to handlers/annotations
# 2. Regenerate swagger docs
make swagger-docs

# 3. Build the application
make build

# 4. Run the server
make run

# 5. Open browser to http://localhost:8080/swagger/index.html
```

## 📚 Additional Resources

- [Swagger UI Official Documentation](https://swagger.io/tools/swagger-ui/)
- [Swagger Annotation Guide](https://github.com/swaggo/swag#general-api-info)
- [OpenAPI Specification](https://swagger.io/specification/)
- [Swaggo GitHub Repository](https://github.com/swaggo/swag)

---

**Need Help?** Check the [README.md](README.md) for general project information or open an issue on GitHub.
