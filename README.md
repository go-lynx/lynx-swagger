# Swagger Plugin for Lynx Framework

A secure and feature-rich Swagger/OpenAPI documentation generator and UI server for the Lynx microservice framework.

## ⚠️ **SECURITY WARNING**

**This plugin is designed for development and testing environments only. It will automatically disable itself in production environments for security reasons.**

## Features

- **Automatic API documentation generation** from Go code annotations
- **Interactive Swagger UI** for API exploration and testing
- **Real-time documentation updates** with file watching
- **Secure by default** with environment-based restrictions
- **Path traversal protection** and file access validation
- **XSS protection** with HTML escaping
- **Secure HTTP headers** and CORS configuration
- **Environment-aware** - automatically disabled in production

## Security Features

### 🔒 **Environment Restrictions**
- Automatically disabled in production environments
- Configurable allowed environments (development, testing only)
- Environment variable detection (`ENV`, `GO_ENV`, `APP_ENV`)

### 🛡️ **Path Security**
- Prevents path traversal attacks
- Restricts file scanning to safe directories
- Validates scan directories against current working directory
- File size limits and type restrictions

### 🌐 **HTTP Security**
- Secure HTTP server configuration with timeouts
- Security headers (X-Frame-Options, X-XSS-Protection, etc.)
- Content Security Policy (CSP)
- Restricted CORS policy (localhost only by default)

### 📝 **Input Validation**
- HTML escaping to prevent XSS attacks
- Safe annotation parsing without regex injection
- Input sanitization and validation

## Installation

```bash
go get github.com/go-lynx/lynx/plugins/swagger
```

## Quick Start

### 1. Import the plugin

```go
import _ "github.com/go-lynx/lynx/plugins/swagger"
```

### 2. Basic configuration

```yaml
lynx:
  swagger:
    enabled: true
    security:
      environment: "development"
    ui:
      enabled: true
      port: 8081
      path: "/swagger"
    generator:
      enabled: true
      scan_dirs: ["./app"]
      output_path: "./docs/openapi.yaml"
```

### 3. Add annotations to your code

```go
// @title My API
// @version 1.0
// @description API documentation

// @Router /users [get]
// @Summary Get users
// @Description Retrieve list of users
// @Param page query int false "Page number"
// @Success 200 {object} []User
func GetUsers(w http.ResponseWriter, r *http.Request) {
    // Your handler code
}
```

## Configuration

### Security Configuration

```yaml
security:
  # Environment detection
  environment: "development"  # Auto-detected from ENV vars
  
  # Allowed environments (Swagger will only run in these)
  allowed_environments:
    - "development"
    - "testing"
  
  # Automatically disable in production
  disable_in_production: true
  
  # Trusted origins for CORS
  trusted_origins:
    - "http://localhost:8080"
    - "http://localhost:8081"
  
  # Require authentication (optional)
  require_auth: false
```

### UI Configuration

`ui.port` is the port for the **Swagger UI page** itself. API requests from "Try it out" are sent to the **lynx-http** server (see `api_server` below).

```yaml
ui:
  enabled: true
  port: 8081                    # Swagger UI page port (separate from lynx-http)
  path: "/swagger"
  title: "API Documentation"
  deep_linking: true
  display_request_duration: true
  doc_expansion: "list"
  default_models_expand_depth: 1
```

### API Server Configuration (Try it out)

Set `api_server` so Swagger UI "Try it out" sends requests to the correct lynx-http address. If not configured, the plugin reads `lynx.http.addr` automatically.

```yaml
api_server:
  host: "localhost:8080"   # lynx-http listen address
  base_path: "/api/v1"     # optional base path
```

| Port | Purpose |
|------|---------|
| `ui.port` (e.g. 8081) | Swagger UI documentation page |
| `api_server.host` (e.g. localhost:8080) | lynx-http API server for "Try it out" |

### Generator Configuration

You can use **one or both** of:

- **External spec files** (`generator.spec_files`): Load OpenAPI/Swagger YAML or JSON (Swagger 2.0 or OpenAPI 3.x, e.g. from `protoc-gen-openapi` in lynx-layout).
- **Go annotation scan** (`generator.scan_dirs`): Scan Go source for `@Router`, `@Summary`, etc.

External specs are loaded first and merged; then annotation scan results are merged. So you can use e.g. `openapi.yaml` from lynx-layout together with annotation-based routes.

```yaml
generator:
  enabled: true
  
  # Optional: load external OpenAPI/Swagger files (YAML or JSON, OAS2 or OAS3)
  spec_files:
    - "./openapi.yaml"
  
  # Directories to scan for Go Swagger annotations
  scan_dirs:
    - "./app/controllers"
    - "./app/handlers"
  
  # Exclude directories
  exclude_dirs:
    - "./vendor"
    - "./test"
    - "./.git"
  
  output_path: "./docs/openapi.yaml"
  watch_enabled: true
  watch_interval: 5s
  gen_on_startup: true
```

### One file: merge `make api` OpenAPI with annotation scan

If you use **lynx-layout** (or any `make api` that runs protoc-gen-openapi), you can have a **single merged file**:

1. **Makefile**: have `make api` output OpenAPI to `docs/` (e.g. `--openapi_out=...:./docs` → `docs/openapi.yaml`).
2. **Config**: set both `spec_files` and `output_path` to that file. At runtime swagger loads it, merges annotation scan, and writes back to the same path.

```yaml
generator:
  enabled: true
  spec_files: ["./docs/openapi.yaml"]   # from make api
  scan_dirs: ["./api", "./internal"]
  output_path: "./docs/openapi.yaml"    # same file = one merged doc
```

Output format: use `.yaml`/`.yml` in `output_path` to write YAML (to match make api); use `.json` to write JSON.

## Environment Variables

The plugin automatically detects the environment from these variables:

```bash
export ENV=development          # Primary environment variable
export GO_ENV=development       # Go-specific environment
export APP_ENV=development      # Application-specific environment
```

## Production Deployment

### Option 1: Explicit Disable

```yaml
lynx:
  swagger:
    enabled: false
```

### Option 2: Environment-Based Disable

```bash
export ENV=production
```

The plugin will automatically detect the production environment and disable itself.

### Option 3: Security Configuration

```yaml
lynx:
  swagger:
    enabled: true
    security:
      environment: "production"
      disable_in_production: true  # Will disable automatically
```

## Security Best Practices

### 1. **Environment Isolation**
- Never run Swagger in production
- Use separate configurations for different environments
- Leverage environment variables for configuration

### 2. **Network Security**
- Bind to localhost only in development
- Use non-privileged ports (>= 1024)
- Restrict CORS to trusted origins only

### 3. **File Access Control**
- Limit scan directories to application code only
- Never scan system directories (`/etc`, `/var`, etc.)
- Validate all file paths before access

### 4. **Input Validation**
- Always escape user input in HTML generation
- Validate configuration parameters
- Use safe parsing methods

## Troubleshooting

### Plugin Won't Start

**Error**: "swagger plugin is not allowed in environment: production"

**Solution**: This is expected behavior. Swagger is automatically disabled in production for security.

### Permission Denied

**Error**: "scanning directory /etc is not allowed for security reasons"

**Solution**: Only scan directories within your application. Never scan system directories.

### Port Already in Use

**Error**: "Failed to start Swagger UI server: address already in use"

**Solution**: Change the port in configuration or stop the conflicting service.

## Examples

See the `example/` directory for complete working examples:

- `full_example.go` - Complete API with annotations
- `swagger-secure.yml` - Secure configuration example

## Contributing

When contributing to this plugin:

1. **Security First**: Always consider security implications
2. **Environment Awareness**: Ensure changes respect environment restrictions
3. **Input Validation**: Validate and sanitize all inputs
4. **Testing**: Test security features thoroughly

## License

Apache 2.0 License - see LICENSE file for details.

## Security Reporting

If you discover a security vulnerability, please report it privately to the maintainers before public disclosure.
