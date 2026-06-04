package swagger

import (
	"path/filepath"
	"testing"

	"github.com/go-lynx/lynx/plugins"
)

func TestSwaggerRuntimeContract_LocalLifecycle(t *testing.T) {
	base := plugins.NewSimpleRuntime()
	rt := base.WithPluginContext(pluginName)

	plugin := NewSwaggerPlugin()
	plugin.rt = rt
	plugin.config = &SwaggerConfig{
		Enabled: true,
		Info: InfoConfig{
			Title:   "Local Swagger",
			Version: "v1.0.0",
		},
		// Explicit environment so the fail-closed disable-gate permits Swagger.
		Security: SecurityConfig{
			Environment: EnvDevelopment,
		},
		UI: UIConfig{
			Path:    "/swagger",
			Enabled: false,
		},
		Gen: GenConfig{
			Enabled:    false,
			OutputPath: filepath.Join(t.TempDir(), "swagger.json"),
		},
	}

	if err := plugin.StartupTasks(); err != nil {
		t.Fatalf("StartupTasks failed: %v", err)
	}

	if alias, err := base.GetSharedResource(sharedPluginResourceName); err != nil || alias != plugin {
		t.Fatalf("unexpected shared plugin alias: value=%#v err=%v", alias, err)
	}
	if readiness, err := base.GetSharedResource(sharedReadinessResourceName); err != nil || readiness != true {
		t.Fatalf("unexpected shared readiness: value=%#v err=%v", readiness, err)
	}
	if health, err := base.GetSharedResource(sharedHealthResourceName); err != nil || health != true {
		t.Fatalf("unexpected shared health: value=%#v err=%v", health, err)
	}
	if _, err := rt.GetPrivateResource("config"); err != nil {
		t.Fatalf("private config resource missing: %v", err)
	}
	if _, err := rt.GetPrivateResource("swagger"); err != nil {
		t.Fatalf("private swagger resource missing: %v", err)
	}

	if err := plugin.CleanupTasks(); err != nil {
		t.Fatalf("CleanupTasks failed: %v", err)
	}

	if readiness, err := base.GetSharedResource(sharedReadinessResourceName); err != nil || readiness != false {
		t.Fatalf("unexpected shared readiness after cleanup: value=%#v err=%v", readiness, err)
	}
	if health, err := base.GetSharedResource(sharedHealthResourceName); err != nil || health != false {
		t.Fatalf("unexpected shared health after cleanup: value=%#v err=%v", health, err)
	}
}
