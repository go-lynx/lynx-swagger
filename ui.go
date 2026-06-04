package swagger

import (
	"fmt"
)

// NOTE: The previous SwaggerUIServer type (and its serveSwaggerJSON stub that
// returned a hardcoded spec, plus a Stop() that used the abrupt server.Close())
// was a dead, divergent serving path. The plugin serves the real merged spec
// via PlugSwagger.startSwaggerUI / serveSwaggerJSON and shuts the server down
// gracefully with Shutdown(ctx) in cleanupWithContext. The dead path has been
// removed to avoid serving a fake spec and abruptly dropping connections.

// GenerateSwaggerUIHTML generates Swagger UI HTML (kept for backward compatibility)
// Deprecated: Use ui.Handler instead
func GenerateSwaggerUIHTML(specURL, title string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>%s</title>
    <link rel="stylesheet" type="text/css" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.9.0/swagger-ui.css">
    <style>
        html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin: 0; background: #fafafa; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.9.0/swagger-ui-bundle.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.9.0/swagger-ui-standalone-preset.js"></script>
    <script>
        window.onload = function() {
            const ui = SwaggerUIBundle({
                url: "%s",
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout"
            });
            window.ui = ui;
        };
    </script>
</body>
</html>`, title, specURL)
}
