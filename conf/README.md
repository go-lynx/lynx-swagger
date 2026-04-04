# Swagger configuration files

This directory contains runtime-aligned Swagger examples:

- `swagger-simple.yml`: smallest working configuration
- `swagger.yml`: fuller runtime example
- `swagger-secure.yml`: development/test-focused example with environment and origin guards

## Live keys

The current runtime reads these sections:

- `enabled`
- `security.environment`
- `security.allowed_environments`
- `security.disable_in_production`
- `security.trusted_origins`
- `generator.enabled`
- `generator.scan_dirs`
- `generator.spec_files`
- `generator.output_path`
- `generator.watch_files`
- `generator.file_watcher.enabled`
- `generator.file_watcher.interval`
- `generator.file_watcher.debounce_delay`
- `generator.file_watcher.max_retries`
- `generator.file_watcher.retry_delay`
- `generator.file_watcher.batch_size`
- `generator.file_watcher.health_check`
- `ui.enabled`
- `ui.port`
- `ui.path`
- `ui.deepLinking`
- `ui.docExpansion`
- `info.title`
- `info.description`
- `info.version`
- `info.termsOfService`
- `info.contact.*`
- `info.license.*`
- `api_server.host`
- `api_server.base_path`

## Parsed but not fully enforced

The current runtime still parses some fields without fully enforcing or rendering them:

- `security.require_auth`
- `ui.displayRequestDuration`
- `ui.defaultModelsExpandDepth`

Those fields are intentionally omitted from the shipped examples so the templates stay production-honest.

## Stale keys removed from the examples

Do not reintroduce template-era keys that the current runtime does not consume:

- `generator.watch_enabled`
- `generator.watch_interval`
- `generator.gen_on_startup`
- `ui.title`
- `ui.spec_url`
- `ui.auto_refresh`
- `ui.refresh_interval`
- snake_case UI keys such as `ui.deep_linking`
- top-level blocks such as `servers`, `schemes`, `consumes`, `produces`, `security_definitions`, and `advanced`

## Operational reminders

- `ui.port` is the port of the Swagger UI server, not the application API server.
- `api_server.host` is the backend address used by Swagger UI "Try it out".
- Production and staging remain blocked by the current runtime guard path.
