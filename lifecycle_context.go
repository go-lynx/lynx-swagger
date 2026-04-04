package swagger

import (
	"context"
	"fmt"

	"github.com/go-lynx/lynx/plugins"
)

func (p *PlugSwagger) InitializeContext(ctx context.Context, plugin plugins.Plugin, rt plugins.Runtime) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("swagger initialize canceled before execution: %w", err)
	}
	return p.BasePlugin.Initialize(plugin, rt)
}

func (p *PlugSwagger) StartContext(ctx context.Context, _ plugins.Plugin) error {
	return p.startupWithContext(ctx)
}

func (p *PlugSwagger) StopContext(ctx context.Context, _ plugins.Plugin) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("swagger stop canceled before execution: %w", err)
	}
	return p.cleanupWithContext(ctx)
}

func (p *PlugSwagger) IsContextAware() bool {
	return true
}

func (p *PlugSwagger) PluginProtocol() plugins.PluginProtocol {
	protocol := p.BasePlugin.PluginProtocol()
	protocol.ContextLifecycle = true
	return protocol
}
