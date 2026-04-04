# Swagger Plugin for Lynx

`lynx-swagger` generates Swagger/OpenAPI documents and serves a separate Swagger UI HTTP endpoint.

## Runtime facts

- Go module: `github.com/go-lynx/lynx-swagger`
- Config prefix: `lynx.swagger`
- Runtime plugin name: `swagger`
- Getter boundary: use the plugin instance and `(*PlugSwagger).GetSwagger()` if you need the in-memory spec

## Current runtime surface

The current runtime reads these YAML sections:

- `lynx.swagger.enabled`
- `lynx.swagger.generator`
- `lynx.swagger.ui`
- `lynx.swagger.info`
- `lynx.swagger.security`
- `lynx.swagger.api_server`

The shipped templates in [`conf`](./conf) now reflect the live keys the runtime actually consumes.

## Minimal working configuration

```yaml
lynx:
  swagger:
    enabled: true
    generator:
      enabled: true
      scan_dirs:
        - "./app"
      output_path: "./docs/openapi.yaml"
      watch_files: true
      file_watcher:
        enabled: true
        interval: 1s
        debounce_delay: 500ms
        max_retries: 3
        retry_delay: 1s
        batch_size: 10
        health_check: true
    ui:
      enabled: true
      port: 8081
      path: "/swagger"
      deepLinking: true
      docExpansion: "list"
    info:
      title: "My API"
      version: "1.0.0"
      description: "Runtime API documentation"
    api_server:
      host: "localhost:8080"
      base_path: "/api/v1"
```

## Important behavior notes

- `generator.watch_files` is the real watcher switch.
- `generator.file_watcher.*` now controls watcher timing and retry behavior.
- `info.title` is the title used both in the generated spec and in the current HTML page.
- `api_server.host` controls where Swagger UI sends "Try it out" requests. If it is empty, the plugin tries to derive it from `lynx.http.addr`.
- The current HTML uses `ui.deepLinking` and `ui.docExpansion`.
- `ui.displayRequestDuration`, `ui.defaultModelsExpandDepth`, and `security.require_auth` are parsed but not enforced/rendered by the current handler.

## Keys you should not rely on

Do not use template-era keys that are not wired into the current runtime, including:

- `generator.watch_enabled`
- `generator.watch_interval`
- `generator.gen_on_startup`
- `ui.title`
- `ui.spec_url`
- `ui.auto_refresh`
- `ui.refresh_interval`
- snake_case UI keys such as `ui.deep_linking` or `ui.doc_expansion`
- top-level Swagger metadata blocks such as `servers`, `schemes`, `consumes`, `produces`, `security_definitions`, and `advanced`

## Environment and production boundary

- By default the plugin is allowed only in `development` and `testing`.
- Production and staging remain blocked by the current runtime guard path.
- `disable_in_production` is effectively treated as `true` by the current defaulting logic, so do not plan on enabling Swagger in production without changing code.
