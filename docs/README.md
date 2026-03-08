# Swagger / OpenAPI Documentation Directory

This directory can store the **merged** API documentation (from `make api` + lynx-swagger annotation scan).

## File names

- `openapi.yaml` – recommended single file: from `make api` (protoc-gen-openapi) and overwritten by lynx-swagger with merged content (proto + annotations).
- `openapi.json` – same content in JSON form if you set `output_path` to a `.json` path.

## How it is generated

1. **make api** (in lynx-layout): writes OpenAPI to `docs/` (e.g. `docs/openapi.yaml`).
2. **lynx-swagger** at runtime: loads that file (`spec_files`), merges annotation scan, and writes back to the same path (`output_path`) so you have one merged file.

## Configuration

```yaml
lynx:
  swagger:
    generator:
      spec_files: ["./docs/openapi.yaml"]
      output_path: "./docs/openapi.yaml"
```
