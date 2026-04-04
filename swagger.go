package swagger

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-lynx/lynx/log"
	"github.com/go-lynx/lynx/plugins"
	"github.com/go-openapi/spec"
	"gopkg.in/yaml.v3"
)

const (
	pluginName        = "swagger"
	pluginVersion     = "v1.6.0-beta"
	pluginDescription = "Swagger API documentation generator and UI server"
	confPrefix        = "lynx.swagger"

	// Security constants
	maxRequestSize = 1 << 20 // 1MB max request size
	readTimeout    = 30 * time.Second
	writeTimeout   = 30 * time.Second
	idleTimeout    = 60 * time.Second
)

// Environment types
const (
	EnvDevelopment = "development"
	EnvTesting     = "testing"
	EnvStaging     = "staging"
	EnvProduction  = "production"
)

// SwaggerConfig plugin configuration
type SwaggerConfig struct {
	Enabled   bool            `json:"enabled" yaml:"enabled"`
	Info      InfoConfig      `json:"info" yaml:"info"`
	UI        UIConfig        `json:"ui" yaml:"ui"`
	Gen       GenConfig       `json:"generator" yaml:"generator"`
	Security  SecurityConfig  `json:"security" yaml:"security"`
	ApiServer ApiServerConfig `json:"api_server" yaml:"api_server"` // API server for Swagger UI "Try it out" (lynx-http address)
}

// ApiServerConfig configures the API server URL used by Swagger UI "Try it out". If empty, reads from lynx.http.addr.
type ApiServerConfig struct {
	Host     string `json:"host" yaml:"host"`           // e.g. "localhost:8080" (lynx-http listen address)
	BasePath string `json:"base_path" yaml:"base_path"` // e.g. "/api/v1"
}

// SecurityConfig security configuration
type SecurityConfig struct {
	Environment    string   `json:"environment" yaml:"environment"`
	AllowedEnvs    []string `json:"allowed_environments" yaml:"allowed_environments"`
	DisableInProd  bool     `json:"disable_in_production" yaml:"disable_in_production"`
	TrustedOrigins []string `json:"trusted_origins" yaml:"trusted_origins"`
	RequireAuth    bool     `json:"require_auth" yaml:"require_auth"`
}

// InfoConfig API basic information
type InfoConfig struct {
	Title          string `json:"title" yaml:"title"`
	Description    string `json:"description" yaml:"description"`
	Version        string `json:"version" yaml:"version"`
	TermsOfService string `json:"termsOfService" yaml:"termsOfService"`
	Contact        struct {
		Name  string `json:"name" yaml:"name"`
		Email string `json:"email" yaml:"email"`
		URL   string `json:"url" yaml:"url"`
	} `json:"contact" yaml:"contact"`
	License struct {
		Name string `json:"name" yaml:"name"`
		URL  string `json:"url" yaml:"url"`
	} `json:"license" yaml:"license"`
}

// UIConfig UI configuration
type UIConfig struct {
	Path                     string `json:"path" yaml:"path"`
	Enabled                  bool   `json:"enabled" yaml:"enabled"`
	DeepLinking              bool   `json:"deepLinking" yaml:"deepLinking"`
	DisplayRequestDuration   bool   `json:"displayRequestDuration" yaml:"displayRequestDuration"`
	DocExpansion             string `json:"docExpansion" yaml:"docExpansion"`
	DefaultModelsExpandDepth int    `json:"defaultModelsExpandDepth" yaml:"defaultModelsExpandDepth"`
	Port                     int    `json:"port" yaml:"port"`
}

// GenConfig generator configuration
type GenConfig struct {
	Enabled     bool              `json:"enabled" yaml:"enabled"`
	ScanDirs    []string          `json:"scan_dirs" yaml:"scan_dirs"`
	SpecFiles   []string          `json:"spec_files" yaml:"spec_files"` // External OpenAPI/Swagger files (YAML or JSON, OAS2 or OAS3)
	OutputPath  string            `json:"output_path" yaml:"output_path"`
	WatchFiles  bool              `json:"watch_files" yaml:"watch_files"`
	FileWatcher FileWatcherConfig `json:"file_watcher" yaml:"file_watcher"`
}

// PlugSwagger Swagger plugin
type PlugSwagger struct {
	*plugins.BasePlugin
	config            *SwaggerConfig
	rt                plugins.Runtime
	swagger           *spec.Swagger
	mu                sync.RWMutex
	server            *http.Server
	generator         *Generator
	watcher           *FileWatcher
	uiServer          *http.Server
	apiServerHost     string // Host:port for API server (lynx-http) used by Swagger UI "Try it out"
	apiServerBasePath string
}

// Generator Swagger documentation generator
type Generator struct {
	config *GenConfig
	parser *Parser
	mu     sync.Mutex
}

// Parser code parser
type Parser struct {
	packages map[string]*Package
	routes   []*Route
}

// Package package information
type Package struct {
	Name    string
	Structs map[string]*Struct
}

// Struct struct information
type Struct struct {
	Name   string
	Fields []Field
	Doc    string
}

// Field field information
type Field struct {
	Name     string
	Type     string
	Tag      string
	Required bool
	Doc      string
}

// Route route information
type Route struct {
	Method      string
	Path        string
	Handler     string
	Summary     string
	Description string
	Tags        []string
	Parameters  []Parameter
	Responses   map[string]Response
}

// Parameter parameter information
type Parameter struct {
	Name        string
	In          string
	Type        string
	Required    bool
	Description string
	Example     string
}

// Response response information
type Response struct {
	Description string
	Schema      interface{}
	Examples    map[string]interface{}
}

// FileWatcherConfig configuration for file watching
type FileWatcherConfig struct {
	Enabled       bool          `json:"enabled" yaml:"enabled"`
	Interval      time.Duration `json:"interval" yaml:"interval"`
	DebounceDelay time.Duration `json:"debounce_delay" yaml:"debounce_delay"`
	MaxRetries    int           `json:"max_retries" yaml:"max_retries"`
	RetryDelay    time.Duration `json:"retry_delay" yaml:"retry_delay"`
	BatchSize     int           `json:"batch_size" yaml:"batch_size"`
	HealthCheck   bool          `json:"health_check" yaml:"health_check"`
}

// FileWatcher enhanced file monitoring
type FileWatcher struct {
	paths       []string
	callback    func() error
	stop        chan struct{}
	stopOnce    sync.Once
	config      FileWatcherConfig
	lastChange  time.Time
	changeCount int
	mu          sync.RWMutex
	healthy     bool
}

func defaultFileWatcherConfig() FileWatcherConfig {
	return FileWatcherConfig{
		Enabled:       true,
		Interval:      1 * time.Second,
		DebounceDelay: 500 * time.Millisecond,
		MaxRetries:    3,
		RetryDelay:    1 * time.Second,
		BatchSize:     10,
		HealthCheck:   true,
	}
}

// NewSwaggerPlugin creates a Swagger plugin
func NewSwaggerPlugin() *PlugSwagger {
	return &PlugSwagger{
		BasePlugin: plugins.NewBasePlugin(
			plugins.GeneratePluginID("", pluginName, pluginVersion),
			pluginName,
			pluginDescription,
			pluginVersion,
			confPrefix,
			100,
		),
		config: &SwaggerConfig{
			Enabled: true,
			UI: UIConfig{
				Path:    "/swagger",
				Enabled: true,
			},
			Gen: GenConfig{
				Enabled:     true,
				ScanDirs:    []string{"./app"},
				OutputPath:  "./docs/openapi.yaml",
				WatchFiles:  true,
				FileWatcher: defaultFileWatcherConfig(),
			},
		},
	}
}

// InitializeResources initializes resources
func (p *PlugSwagger) InitializeResources(rt plugins.Runtime) error {
	if err := p.BasePlugin.InitializeResources(rt); err != nil {
		return err
	}
	p.rt = rt
	// Load configuration
	if err := rt.GetConfig().Value(confPrefix).Scan(p.config); err != nil {
		log.Warnf("Failed to load swagger config, using defaults: %v", err)
	}

	// Set default values
	if p.config.Info.Title == "" {
		p.config.Info.Title = "API Documentation"
	}
	if p.config.Info.Version == "" {
		p.config.Info.Version = "1.0.0"
	}
	if p.config.UI.Path == "" {
		p.config.UI.Path = "/swagger"
	}
	if p.config.Gen.OutputPath == "" {
		p.config.Gen.OutputPath = "./docs/openapi.yaml"
	}

	// Set default values
	p.SetDefaultValues()

	// Initialize generator
	p.generator = &Generator{
		config: &p.config.Gen,
		parser: &Parser{
			packages: make(map[string]*Package),
			routes:   make([]*Route, 0),
		},
	}

	// Initialize Swagger specification
	p.swagger = p.newBaseSwaggerSpec()

	// Derive API server (host:port) for Swagger UI "Try it out" from explicit config or lynx.http
	p.deriveApiServerHost(rt)

	return nil
}

func (p *PlugSwagger) newBaseSwaggerSpec() *spec.Swagger {
	swaggerDoc := &spec.Swagger{
		SwaggerProps: spec.SwaggerProps{
			Swagger: "2.0",
			Info: &spec.Info{
				InfoProps: spec.InfoProps{
					Title:          p.config.Info.Title,
					Description:    p.config.Info.Description,
					Version:        p.config.Info.Version,
					TermsOfService: p.config.Info.TermsOfService,
				},
			},
			Schemes:     []string{"http", "https"},
			Paths:       &spec.Paths{Paths: make(map[string]spec.PathItem)},
			Definitions: spec.Definitions{},
		},
	}

	// Set contact information
	if p.config.Info.Contact.Name != "" {
		swaggerDoc.Info.Contact = &spec.ContactInfo{
			ContactInfoProps: spec.ContactInfoProps{},
		}
	}

	// Set license information
	if p.config.Info.License.Name != "" {
		swaggerDoc.Info.License = &spec.License{
			LicenseProps: spec.LicenseProps{},
		}
	}

	return swaggerDoc
}

// StartupTasks startup tasks
func (p *PlugSwagger) StartupTasks() error {
	return p.startupWithContext(context.Background())
}

func (p *PlugSwagger) startupWithContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("swagger startup canceled before execution: %w", err)
	}
	if p.rt != nil {
		if err := p.rt.RegisterSharedResource(pluginName, p); err != nil {
			p.publishRuntimeContract(false, false)
			return fmt.Errorf("failed to register Swagger shared resource: %w", err)
		}
		p.registerRuntimePluginAlias()
		if err := p.rt.RegisterPrivateResource("config", p.config); err != nil {
			log.Warnf("failed to register Swagger private config resource: %v", err)
		}
	}
	if !p.config.Enabled {
		log.Info("Swagger plugin is disabled")
		p.publishRuntimeContract(false, true)
		return nil
	}
	p.publishRuntimeContract(false, false)

	// Validate configuration
	if err := p.validateConfiguration(); err != nil {
		p.publishRuntimeContract(false, false)
		return fmt.Errorf("invalid swagger configuration: %w", err)
	}

	if err := p.rebuildSwaggerDocs(); err != nil {
		p.publishRuntimeContract(false, false)
		return fmt.Errorf("build swagger doc: %w", err)
	}

	// 5) Save merged doc and register
	if err := p.saveSwaggerJSON(); err != nil {
		p.publishRuntimeContract(false, false)
		return fmt.Errorf("save swagger json: %w", err)
	}
	if err := p.registerSwaggerToSwag(); err != nil {
		p.publishRuntimeContract(false, false)
		return fmt.Errorf("register swagger: %w", err)
	}
	log.Info("Swagger documentation generated successfully")

	// Start UI service
	if p.config.UI.Enabled {
		r := p.startSwaggerUI()
		if r != nil {
			p.publishRuntimeContract(false, false)
			return fmt.Errorf("failed to start swagger UI server: %w", r)
		}
	}

	// Start file monitoring (only when using annotation scan)
	if p.shouldStartFileWatcher() {
		p.startFileWatcher()
	}

	if p.rt != nil {
		if p.generator != nil {
			if err := p.rt.RegisterPrivateResource("generator", p.generator); err != nil {
				log.Warnf("failed to register Swagger private generator resource: %v", err)
			}
		}
		if p.swagger != nil {
			if err := p.rt.RegisterPrivateResource("swagger", p.swagger); err != nil {
				log.Warnf("failed to register Swagger private spec resource: %v", err)
			}
		}
		if p.uiServer != nil {
			if err := p.rt.RegisterPrivateResource("ui_server", p.uiServer); err != nil {
				log.Warnf("failed to register Swagger private UI server resource: %v", err)
			}
		}
		if p.watcher != nil {
			if err := p.rt.RegisterPrivateResource("file_watcher", p.watcher); err != nil {
				log.Warnf("failed to register Swagger private file watcher resource: %v", err)
			}
		}
	}

	// Log that plugin has started
	log.Infof("Swagger plugin registered to application")

	if err := p.CheckHealth(); err != nil {
		p.publishRuntimeContract(false, false)
		return err
	}
	p.publishRuntimeContract(true, true)

	log.Infof("Swagger plugin started successfully. UI available at %s", p.config.UI.Path)
	return nil
}

func (p *PlugSwagger) rebuildSwaggerDocs() error {
	p.UpdateSwagger(p.newBaseSwaggerSpec())

	// 1) Load external OpenAPI/Swagger files (e.g. openapi.yaml from protoc-gen-openapi) and merge into spec.
	if err := p.loadAndMergeExternalSpecs(); err != nil {
		return fmt.Errorf("load external specs: %w", err)
	}

	// 2) Optionally scan Go annotations and merge into the same in-memory doc.
	if p.config.Gen.Enabled && len(p.config.Gen.ScanDirs) > 0 {
		if err := p.generateSwaggerDocs(); err != nil {
			log.Errorf("Failed to generate swagger docs from annotations: %v", err)
		}
	}

	// 3) Ensure we have at least one path (e.g. placeholder) when serving UI.
	p.ensurePathsOrDefault()

	// 4) Apply API server host (lynx-http) to spec so Swagger UI "Try it out" sends requests to correct server.
	p.applyApiServerToSpec()

	return nil
}

// CleanupTasks cleanup tasks
func (p *PlugSwagger) CleanupTasks() error {
	return p.cleanupWithContext(context.Background())
}

func (p *PlugSwagger) cleanupWithContext(parentCtx context.Context) error {
	if err := parentCtx.Err(); err != nil {
		return fmt.Errorf("swagger cleanup canceled before execution: %w", err)
	}
	p.publishRuntimeContract(false, false)
	// Stop file monitoring
	if p.watcher != nil {
		p.watcher.stopWatching()
	}

	// Stop UI server
	if p.uiServer != nil {
		ctx, cancel := context.WithTimeout(parentCtx, 5*time.Second)
		defer cancel()
		if err := p.uiServer.Shutdown(ctx); err != nil {
			log.Errorf("Failed to shutdown swagger UI server: %v", err)
		}
		p.uiServer = nil
	}

	return nil
}

// generateSwaggerDocs scans Go annotations and merges routes into p.swagger. Does not save or register.
func (p *PlugSwagger) generateSwaggerDocs() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	log.Info("Starting Swagger documentation generation from Go annotations...")

	// Create annotation parser with allowed directories
	parser := NewAnnotationParser(p.swagger, p.config.Gen.ScanDirs)

	// Scan directories
	for _, dir := range p.config.Gen.ScanDirs {
		log.Infof("Scanning directory: %s", dir)
		if err := parser.ScanDirectory(dir); err != nil {
			log.Warnf("Failed to scan directory %s: %v", dir, err)
		}
	}

	return nil
}

// ensurePathsOrDefault adds a default /health path when no paths exist.
func (p *PlugSwagger) ensurePathsOrDefault() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.swagger.Paths != nil && len(p.swagger.Paths.Paths) > 0 {
		return
	}
	p.swagger.Paths = &spec.Paths{
		Paths: map[string]spec.PathItem{
			"/health": {
				PathItemProps: spec.PathItemProps{
					Get: &spec.Operation{
						OperationProps: spec.OperationProps{
							ID:          "health-check",
							Summary:     "Health Check",
							Description: "Check the health status of the service",
							Tags:        []string{"Health"},
							Responses: &spec.Responses{
								ResponsesProps: spec.ResponsesProps{
									StatusCodeResponses: map[int]spec.Response{
										200: {
											ResponseProps: spec.ResponseProps{
												Description: "Service is healthy",
												Schema: &spec.Schema{
													SchemaProps: spec.SchemaProps{
														Type: []string{"object"},
														Properties: map[string]spec.Schema{
															"status": {
																SchemaProps: spec.SchemaProps{
																	Type: []string{"string"},
																},
															},
															"timestamp": {
																SchemaProps: spec.SchemaProps{
																	Type:   []string{"string"},
																	Format: "date-time",
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// startSwaggerUI starts Swagger UI service
func (p *PlugSwagger) startSwaggerUI() error {
	if !p.config.UI.Enabled {
		return nil
	}

	// Create independent HTTP server for Swagger UI
	swaggerPath := p.config.UI.Path
	if swaggerPath == "" {
		swaggerPath = "/swagger"
	}

	// Create HTTP multiplexer
	mux := http.NewServeMux()

	// Register JSON documentation route
	mux.HandleFunc(swaggerPath+"/doc.json", p.serveSwaggerJSON)
	mux.HandleFunc("/swagger.json", p.serveSwaggerJSON)
	mux.HandleFunc("/api-docs", p.serveSwaggerJSON)

	// Register Swagger UI static file service
	// Note: Simplified handling here, actual Swagger UI static files need to be embedded
	mux.HandleFunc(swaggerPath+"/", p.serveSwaggerUI)

	// Get port configuration
	port := p.config.UI.Port
	if port == 0 {
		port = 8081 // Default port
	}

	// Create HTTP server with security configuration
	p.uiServer = &http.Server{
		Addr:           fmt.Sprintf(":%d", port),
		Handler:        p.createSecureHandler(mux),
		ReadTimeout:    readTimeout,
		WriteTimeout:   writeTimeout,
		IdleTimeout:    idleTimeout,
		MaxHeaderBytes: maxRequestSize,
	}

	listener, err := net.Listen("tcp", p.uiServer.Addr)
	if err != nil {
		return err
	}

	// Start server in the background
	go func() {
		log.Infof("Starting Swagger UI server on http://%s%s", listener.Addr().String(), swaggerPath)
		if err := p.uiServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Errorf("Failed to start Swagger UI server: %v", err)
		}
	}()

	return nil
}

// createSecureHandler creates a secure HTTP handler with security headers
func (p *PlugSwagger) createSecureHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' cdn.jsdelivr.net; font-src 'self' cdn.jsdelivr.net; img-src 'self' data:;")

		// Add CORS headers with restricted origins
		if p.isCORSAllowed(r.Header.Get("Origin")) {
			w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "null")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "3600")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Check request size
		if r.ContentLength > maxRequestSize {
			http.Error(w, "Request too large", http.StatusRequestEntityTooLarge)
			return
		}

		// Call the actual handler
		handler.ServeHTTP(w, r)
	})
}

// isCORSAllowed checks if the origin is allowed for CORS
func (p *PlugSwagger) isCORSAllowed(origin string) bool {
	// If no trusted origins configured, only allow localhost
	if len(p.config.Security.TrustedOrigins) == 0 {
		return strings.HasPrefix(origin, "http://localhost:") ||
			strings.HasPrefix(origin, "https://localhost:") ||
			strings.HasPrefix(origin, "http://127.0.0.1:") ||
			strings.HasPrefix(origin, "https://127.0.0.1:")
	}

	// Check against configured trusted origins
	for _, trusted := range p.config.Security.TrustedOrigins {
		if origin == trusted {
			return true
		}
	}

	return false
}

// serveSwaggerJSON provides Swagger JSON documentation
func (p *PlugSwagger) serveSwaggerJSON(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	// CORS headers are now handled by createSecureHandler

	if err := json.NewEncoder(w).Encode(p.swagger); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// serveSwaggerUI provides Swagger UI page
func (p *PlugSwagger) serveSwaggerUI(w http.ResponseWriter, r *http.Request) {
	// Escape user input to prevent XSS
	title := p.escapeHTML(p.config.Info.Title)
	path := p.escapeHTML(p.config.UI.Path)
	deepLinking := p.config.UI.DeepLinking
	docExpansion := p.escapeHTML(p.config.UI.DocExpansion)

	// Simple Swagger UI HTML page with escaped content
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>` + title + ` - Swagger UI</title>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
    <style>
        body { margin: 0; padding: 0; }
        #swagger-ui { padding: 20px; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
    <script>
        window.onload = function() {
            window.ui = SwaggerUIBundle({
                url: "` + path + `/doc.json",
                dom_id: '#swagger-ui',
                deepLinking: ` + fmt.Sprintf("%v", deepLinking) + `,
                docExpansion: "` + docExpansion + `",
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                layout: "StandaloneLayout"
            });
        };
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// escapeHTML escapes HTML special characters to prevent XSS
func (p *PlugSwagger) escapeHTML(s string) string {
	escaped := strings.ReplaceAll(s, "&", "&amp;")
	escaped = strings.ReplaceAll(escaped, "<", "&lt;")
	escaped = strings.ReplaceAll(escaped, ">", "&gt;")
	escaped = strings.ReplaceAll(escaped, `"`, "&quot;")
	escaped = strings.ReplaceAll(escaped, "'", "&#39;")
	return escaped
}

// startFileWatcher starts file monitoring
func (p *PlugSwagger) startFileWatcher() {
	fileWatcherConfig := p.fileWatcherConfig()
	if !fileWatcherConfig.Enabled {
		return
	}

	p.watcher = &FileWatcher{
		paths: p.config.Gen.ScanDirs,
		callback: func() error {
			if err := p.rebuildSwaggerDocs(); err != nil {
				log.Errorf("Failed to rebuild docs: %v", err)
				return err
			}
			// Persist updated spec to output_path
			if err := p.saveSwaggerJSON(); err != nil {
				log.Errorf("Failed to save swagger json after change: %v", err)
				return err
			}
			return nil
		},
		config:      fileWatcherConfig,
		lastChange:  time.Now(),
		changeCount: 0,
		stop:        make(chan struct{}),
		mu:          sync.RWMutex{},
		healthy:     true,
	}

	go p.watcher.watch()
}

func (w *FileWatcher) stopWatching() {
	if w == nil || w.stop == nil {
		return
	}
	w.stopOnce.Do(func() {
		close(w.stop)
	})
}

// watch monitors file changes with enhanced logic
func (w *FileWatcher) watch() {
	ticker := time.NewTicker(w.config.Interval)
	defer ticker.Stop()

	debounceTimer := time.NewTimer(w.config.DebounceDelay)
	debounceTimer.Stop()

	for {
		select {
		case <-ticker.C:
			if w.checkForChanges() {
				// Reset debounce timer
				debounceTimer.Reset(w.config.DebounceDelay)
			}
		case <-debounceTimer.C:
			// Debounced change processing
			w.processChanges()
		case <-w.stop:
			debounceTimer.Stop()
			return
		}
	}
}

// checkForChanges checks if any files have changed
func (w *FileWatcher) checkForChanges() bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Simple file modification check
	for _, path := range w.paths {
		if info, err := os.Stat(path); err == nil {
			if info.ModTime().After(w.lastChange) {
				w.lastChange = info.ModTime()
				w.changeCount++
				return true
			}
		}
	}
	return false
}

// processChanges processes file changes with retry logic
func (w *FileWatcher) processChanges() {
	w.mu.Lock()
	changeCount := w.changeCount
	w.changeCount = 0
	w.mu.Unlock()

	if changeCount == 0 {
		return
	}

	// Retry logic with exponential backoff
	for attempt := 0; attempt < w.config.MaxRetries; attempt++ {
		if err := w.callback(); err == nil {
			w.mu.Lock()
			w.healthy = true
			w.mu.Unlock()
			log.Infof("Successfully processed %d file changes", changeCount)
			return
		}

		if attempt < w.config.MaxRetries-1 {
			delay := w.config.RetryDelay * time.Duration(1<<attempt)
			log.Warnf("Failed to process changes (attempt %d/%d), retrying in %v",
				attempt+1, w.config.MaxRetries, delay)
			retryTimer := time.NewTimer(delay)
			select {
			case <-w.stop:
				if !retryTimer.Stop() {
					<-retryTimer.C
				}
				return
			case <-retryTimer.C:
			}
		}
	}

	// All retries failed
	w.mu.Lock()
	w.healthy = false
	w.mu.Unlock()
	log.Errorf("Failed to process file changes after %d attempts", w.config.MaxRetries)
}

// IsHealthy returns the health status of the file watcher
func (w *FileWatcher) IsHealthy() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.healthy
}

// GetStats returns file watcher statistics
func (w *FileWatcher) GetStats() map[string]interface{} {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return map[string]interface{}{
		"healthy":      w.healthy,
		"change_count": w.changeCount,
		"last_change":  w.lastChange,
		"paths":        w.paths,
	}
}

// saveSwaggerJSON saves Swagger documentation to output_path. Writes YAML if path ends with .yaml/.yml, else JSON.
func (p *PlugSwagger) saveSwaggerJSON() error {
	if p.config.Gen.OutputPath == "" {
		return nil
	}

	dir := filepath.Dir(p.config.Gen.OutputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(p.config.Gen.OutputPath))
	isYAML := ext == ".yaml" || ext == ".yml"

	var data []byte
	var err error
	if isYAML {
		// Marshal to JSON then to map so we can output YAML (spec.Swagger has json tags)
		var raw map[string]interface{}
		jsonBytes, err := json.Marshal(p.swagger)
		if err != nil {
			return fmt.Errorf("failed to marshal swagger: %w", err)
		}
		if err = json.Unmarshal(jsonBytes, &raw); err != nil {
			return fmt.Errorf("failed to convert to map: %w", err)
		}
		data, err = yaml.Marshal(raw)
		if err != nil {
			return fmt.Errorf("failed to marshal yaml: %w", err)
		}
	} else {
		data, err = json.MarshalIndent(p.swagger, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal swagger: %w", err)
		}
	}

	if err := os.WriteFile(p.config.Gen.OutputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write swagger file: %w", err)
	}

	log.Infof("Swagger doc saved to %s", p.config.Gen.OutputPath)
	return nil
}

// registerSwaggerToSwag registers Swagger documentation to internal registry
func (p *PlugSwagger) registerSwaggerToSwag() error {
	// Simplified handling here, actual project can integrate swaggo/swag package
	// Or use other methods to manage Swagger documentation

	log.Infof("Swagger specification registered successfully")
	return nil
}

// Health health check
func (p *PlugSwagger) Health() error {
	return nil
}

// swagDoc Swagger documentation implementation
type swagDoc struct {
	swagger *spec.Swagger
}

func (s *swagDoc) ReadDoc() string {
	data, _ := json.Marshal(s.swagger)
	return string(data)
}

// GetSwagger gets Swagger specification
func (p *PlugSwagger) GetSwagger() *spec.Swagger {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.swagger
}

// UpdateSwagger updates Swagger specification
func (p *PlugSwagger) UpdateSwagger(swagger *spec.Swagger) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.swagger = swagger
}

// AddPath adds path
func (p *PlugSwagger) AddPath(path string, item spec.PathItem) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.swagger.Paths == nil {
		p.swagger.Paths = &spec.Paths{Paths: make(map[string]spec.PathItem)}
	}
	p.swagger.Paths.Paths[path] = item
}

// AddDefinition adds definition
func (p *PlugSwagger) AddDefinition(name string, schema spec.Schema) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.swagger.Definitions == nil {
		p.swagger.Definitions = make(spec.Definitions)
	}
	p.swagger.Definitions[name] = schema
}

// CheckHealth health check
func (p *PlugSwagger) CheckHealth() error {
	if !p.config.Enabled {
		return nil
	}

	// Check if documentation is generated
	if p.swagger == nil || p.swagger.Paths == nil {
		return fmt.Errorf("swagger documentation not generated")
	}
	if p.config.UI.Enabled && p.uiServer == nil {
		return fmt.Errorf("swagger UI server not initialized")
	}
	if p.shouldStartFileWatcher() && p.watcher == nil {
		return fmt.Errorf("swagger file watcher not initialized")
	}
	if p.watcher != nil && !p.watcher.IsHealthy() {
		return fmt.Errorf("swagger file watcher is unhealthy")
	}

	return nil
}

// isEnvironmentAllowed checks if the current environment allows Swagger to run
func (p *PlugSwagger) isEnvironmentAllowed() bool {
	// Check if explicitly disabled in production
	if p.config.Security.DisableInProd && p.isProductionEnvironment() {
		return false
	}

	// Check allowed environments list
	if len(p.config.Security.AllowedEnvs) > 0 {
		currentEnv := p.getCurrentEnvironment()
		for _, allowed := range p.config.Security.AllowedEnvs {
			if currentEnv == allowed {
				return true
			}
		}
		return false
	}

	// Default: allow in development and testing, deny in production
	return !p.isProductionEnvironment()
}

// isProductionEnvironment checks if current environment is production
func (p *PlugSwagger) isProductionEnvironment() bool {
	env := p.getCurrentEnvironment()
	return env == EnvProduction || env == EnvStaging
}

// getCurrentEnvironment gets current environment
func (p *PlugSwagger) getCurrentEnvironment() string {
	// Check environment variable first
	if env := os.Getenv("ENV"); env != "" {
		return strings.ToLower(env)
	}
	if env := os.Getenv("GO_ENV"); env != "" {
		return strings.ToLower(env)
	}
	if env := os.Getenv("APP_ENV"); env != "" {
		return strings.ToLower(env)
	}

	// Check config
	if p.config.Security.Environment != "" {
		return strings.ToLower(p.config.Security.Environment)
	}

	// Default to development
	return EnvDevelopment
}

// validateConfiguration validates configuration for security
func (p *PlugSwagger) validateConfiguration() error {
	// Check if environment allows Swagger
	if !p.isEnvironmentAllowed() {
		return fmt.Errorf("swagger plugin is not allowed in environment: %s", p.getCurrentEnvironment())
	}

	// Validate at least one source: external spec files or scan directories (when generator enabled)
	specFiles := p.resolvedSpecFiles()
	hasSpecFiles := len(specFiles) > 0
	hasScanDirs := len(p.config.Gen.ScanDirs) > 0
	if !hasSpecFiles && (!p.config.Gen.Enabled || !hasScanDirs) {
		// When generator enabled, need either spec_files or scan_dirs
		if p.config.Gen.Enabled {
			return fmt.Errorf("invalid swagger configuration: either generator.spec_files or generator.scan_dirs must be set when generator.enabled is true")
		}
		// When generator disabled, need spec_files to have any doc to show
		if p.config.UI.Enabled && !hasSpecFiles {
			return fmt.Errorf("invalid swagger configuration: generator.spec_files is required when generator.enabled is false and UI is enabled")
		}
	}

	// Validate external spec files (path exists and within cwd)
	for _, f := range specFiles {
		if err := p.validateSpecFile(f); err != nil {
			return fmt.Errorf("invalid spec file %s: %w", f, err)
		}
	}

	// Validate scan directories only when generator is enabled and scan_dirs are used
	if p.config.Gen.Enabled && len(p.config.Gen.ScanDirs) > 0 {
		for _, dir := range p.config.Gen.ScanDirs {
			if err := p.validateScanDirectory(dir); err != nil {
				return fmt.Errorf("invalid scan directory %s: %w", dir, err)
			}
		}
	}

	// Validate UI configuration
	if p.config.UI.Enabled {
		if err := p.validateUIConfig(); err != nil {
			return fmt.Errorf("invalid UI configuration: %w", err)
		}
	}

	return nil
}

// validateSpecFile validates spec file path for security
func (p *PlugSwagger) validateSpecFile(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", absPath)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}
	if !strings.HasPrefix(absPath, cwd) {
		return fmt.Errorf("spec file must be within current working directory %s", cwd)
	}
	return nil
}

// deriveApiServerHost derives API server host:port from explicit config or lynx.http.addr.
// Called in InitializeResources. Result is stored in p.apiServerHost and p.apiServerBasePath.
func (p *PlugSwagger) deriveApiServerHost(rt plugins.Runtime) {
	if p.config.ApiServer.Host != "" {
		p.apiServerHost = p.config.ApiServer.Host
		p.apiServerBasePath = p.config.ApiServer.BasePath
		return
	}
	// Fallback: read lynx.http.addr
	var httpConf struct {
		Addr string `json:"addr" yaml:"addr"`
	}
	if err := rt.GetConfig().Value("lynx.http").Scan(&httpConf); err != nil || httpConf.Addr == "" {
		log.Debugf("Swagger: could not read lynx.http.addr (use api_server.host to configure): %v", err)
		return
	}
	addr := httpConf.Addr
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		log.Warnf("Swagger: invalid lynx.http.addr %q: %v", httpConf.Addr, err)
		return
	}
	// For ":8080" or "0.0.0.0:8080", use localhost for Try it out (same-machine testing)
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}
	p.apiServerHost = net.JoinHostPort(host, port)
	p.apiServerBasePath = p.config.ApiServer.BasePath
	log.Infof("Swagger: API server for Try it out derived from lynx.http: %s%s", p.apiServerHost, p.apiServerBasePath)
}

// applyApiServerToSpec sets p.swagger.Host and BasePath for Swagger UI "Try it out" requests.
func (p *PlugSwagger) applyApiServerToSpec() {
	if p.apiServerHost == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.swagger.Host = p.apiServerHost
	if p.apiServerBasePath != "" {
		p.swagger.BasePath = p.apiServerBasePath
	}
}

// validateScanDirectory validates scan directory for security
func (p *PlugSwagger) validateScanDirectory(dir string) error {
	// Convert to absolute path
	absPath, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if directory exists
	if _, statErr := os.Stat(absPath); os.IsNotExist(statErr) {
		return fmt.Errorf("directory does not exist: %s", absPath)
	}

	// Check for suspicious paths
	suspicious := []string{"/etc", "/var", "/usr", "/bin", "/sbin", "/tmp", "/root", "/home"}
	for _, s := range suspicious {
		if strings.HasPrefix(absPath, s) {
			return fmt.Errorf("scanning directory %s is not allowed for security reasons", absPath)
		}
	}

	// Check if it's a subdirectory of current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}

	if !strings.HasPrefix(absPath, cwd) {
		return fmt.Errorf("scan directory %s must be within current working directory %s", absPath, cwd)
	}

	return nil
}

// validateUIConfig validates UI configuration for security
func (p *PlugSwagger) validateUIConfig() error {
	// Check port range
	if p.config.UI.Port < 1024 || p.config.UI.Port > 65535 {
		return fmt.Errorf("UI port must be between 1024 and 65535, got: %d", p.config.UI.Port)
	}

	// Check path
	if p.config.UI.Path == "" {
		return fmt.Errorf("UI path cannot be empty")
	}

	// Ensure path starts with /
	if !strings.HasPrefix(p.config.UI.Path, "/") {
		return fmt.Errorf("UI path must start with /, got: %s", p.config.UI.Path)
	}

	return nil
}

// SetDefaultValues sets default values for configuration
func (p *PlugSwagger) SetDefaultValues() {
	// Set default security configuration
	if p.config.Security.Environment == "" {
		p.config.Security.Environment = EnvDevelopment
	}

	// Set default allowed environments (development and testing only)
	if len(p.config.Security.AllowedEnvs) == 0 {
		p.config.Security.AllowedEnvs = []string{EnvDevelopment, EnvTesting}
	}

	// Default to disable in production
	if !p.config.Security.DisableInProd {
		p.config.Security.DisableInProd = true
	}

	// Set default trusted origins (localhost only)
	if len(p.config.Security.TrustedOrigins) == 0 {
		p.config.Security.TrustedOrigins = []string{
			"http://localhost:8080",
			"http://localhost:8081",
			"http://127.0.0.1:8080",
			"http://127.0.0.1:8081",
		}
	}

	// Set default UI configuration
	if p.config.UI.Path == "" {
		p.config.UI.Path = "/swagger"
	}

	if p.config.UI.Port == 0 {
		p.config.UI.Port = 8081
	}

	// Set default generator configuration
	if len(p.config.Gen.ScanDirs) == 0 {
		p.config.Gen.ScanDirs = []string{"./"}
	}

	if p.config.Gen.OutputPath == "" {
		p.config.Gen.OutputPath = "./docs/openapi.yaml"
	}

	p.config.Gen.FileWatcher = p.fileWatcherConfig()
}

func (p *PlugSwagger) shouldStartFileWatcher() bool {
	return p.config.Gen.WatchFiles && p.config.Gen.Enabled && len(p.config.Gen.ScanDirs) > 0 && p.fileWatcherConfig().Enabled
}

func (p *PlugSwagger) fileWatcherConfig() FileWatcherConfig {
	cfg := p.config.Gen.FileWatcher
	defaults := defaultFileWatcherConfig()

	if cfg.Interval <= 0 {
		cfg.Interval = defaults.Interval
	}
	if cfg.DebounceDelay <= 0 {
		cfg.DebounceDelay = defaults.DebounceDelay
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = defaults.MaxRetries
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = defaults.RetryDelay
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaults.BatchSize
	}

	return cfg
}
