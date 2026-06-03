package swagger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-lynx/lynx/log"
	"github.com/go-openapi/spec"
	"gopkg.in/yaml.v3"
)

// loadAndMergeExternalSpecs loads all generator.spec_files and merges them into p.swagger.
func (p *PlugSwagger) loadAndMergeExternalSpecs() error {
	specFiles := p.resolvedSpecFiles()
	if len(specFiles) == 0 {
		return nil
	}
	for _, path := range specFiles {
		loaded, err := loadSpecFile(path)
		if err != nil {
			return fmt.Errorf("load spec file %s: %w", path, err)
		}
		mergeSpecInto(p.swagger, loaded)
		log.Infof("Loaded and merged external spec: %s", path)
	}
	return nil
}

func (p *PlugSwagger) resolvedSpecFiles() []string {
	seen := make(map[string]struct{}, len(p.config.Gen.SpecFiles)+1)
	files := make([]string, 0, len(p.config.Gen.SpecFiles)+1)
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		cleaned := filepath.Clean(path)
		key := cleaned
		if abs, err := filepath.Abs(cleaned); err == nil {
			key = abs
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		files = append(files, cleaned)
	}

	for _, path := range p.config.Gen.SpecFiles {
		add(path)
	}

	// Compatibility mode: if the project already has docs/openapi.yaml and the
	// user did not explicitly configure spec_files, treat the existing output file
	// as the baseline protobuf/OpenAPI document and merge annotation scan into it.
	if len(files) == 0 {
		outputPath := strings.TrimSpace(p.config.Gen.OutputPath)
		if outputPath != "" {
			if info, err := os.Stat(outputPath); err == nil && !info.IsDir() {
				add(outputPath)
			}
		}
	}

	return files
}

// loadSpecFile reads a single file (YAML or JSON), detects OAS2 vs OAS3, and returns *spec.Swagger.
func loadSpecFile(path string) (*spec.Swagger, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	isYAML := ext == ".yaml" || ext == ".yml"

	// Detect version from first bytes (swagger 2.0 has "swagger": "2.0", OAS3 has "openapi": "3.x")
	var raw map[string]any
	if isYAML {
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("yaml unmarshal: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("json unmarshal: %w", err)
		}
	}

	if raw == nil {
		return nil, fmt.Errorf("empty spec")
	}

	// OpenAPI 3.x
	if oapiVer, ok := raw["openapi"]; ok {
		verStr, _ := oapiVer.(string)
		if strings.HasPrefix(verStr, "3.") {
			return loadOAS3AndConvertToSwagger2(data, path)
		}
	}

	// Swagger 2.0: convert to spec.Swagger via JSON roundtrip so json tags are respected
	var swagger2 spec.Swagger
	if isYAML {
		jsonBytes, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("yaml->json: %w", err)
		}
		if err := json.Unmarshal(jsonBytes, &swagger2); err != nil {
			return nil, fmt.Errorf("swagger2 unmarshal: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, &swagger2); err != nil {
			return nil, fmt.Errorf("swagger2 unmarshal: %w", err)
		}
	}
	return &swagger2, nil
}

// loadOAS3AndConvertToSwagger2 loads OpenAPI 3 spec and converts to go-openapi spec.Swagger (Swagger 2.0).
func loadOAS3AndConvertToSwagger2(data []byte, path string) (*spec.Swagger, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(data)
	if err != nil {
		return nil, fmt.Errorf("openapi3 load: %w", err)
	}
	// Resolve refs so conversion sees full document
	if err := doc.Validate(loader.Context); err != nil {
		log.Warnf("OpenAPI 3 validation warning for %s: %v", path, err)
	}
	swagger2, err := openapi2conv.FromV3(doc)
	if err != nil {
		return nil, fmt.Errorf("openapi3->swagger2: %w", err)
	}
	// kin-openapi openapi2.T -> go-openapi spec.Swagger via JSON
	jsonBytes, err := json.Marshal(swagger2)
	if err != nil {
		return nil, fmt.Errorf("swagger2 marshal: %w", err)
	}
	var out spec.Swagger
	if err := json.Unmarshal(jsonBytes, &out); err != nil {
		return nil, fmt.Errorf("spec.Swagger unmarshal: %w", err)
	}
	return &out, nil
}

// mergeSpecInto merges paths and definitions from src into dst (dst is modified).
func mergeSpecInto(dst, src *spec.Swagger) {
	if src == nil {
		return
	}
	if src.Paths != nil && len(src.Paths.Paths) > 0 {
		if dst.Paths == nil {
			dst.Paths = &spec.Paths{Paths: make(map[string]spec.PathItem)}
		}
		for path, item := range src.Paths.Paths {
			dst.Paths.Paths[path] = item
		}
	}
	if len(src.Definitions) > 0 {
		if len(dst.Definitions) == 0 {
			dst.Definitions = make(spec.Definitions)
		}
		for name, def := range src.Definitions {
			dst.Definitions[name] = def
		}
	}
	// Optionally merge info from first external spec if dst has defaults only
	if src.Info != nil && src.Info.Title != "" && (dst.Info == nil || dst.Info.Title == "") {
		if dst.Info == nil {
			dst.Info = &spec.Info{}
		}
		if dst.Info.Title == "" {
			dst.Info.Title = src.Info.Title
		}
		if dst.Info.Description == "" && src.Info.Description != "" {
			dst.Info.Description = src.Info.Description
		}
		if dst.Info.Version == "" && src.Info.Version != "" {
			dst.Info.Version = src.Info.Version
		}
	}
}
