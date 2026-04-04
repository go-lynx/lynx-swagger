package swagger

import "github.com/go-lynx/lynx/log"

const (
	sharedPluginResourceName     = pluginName + ".plugin"
	sharedReadinessResourceName  = pluginName + ".readiness"
	sharedHealthResourceName     = pluginName + ".health"
	privateReadinessResourceName = "readiness"
	privateHealthResourceName    = "health"
)

func (p *PlugSwagger) registerRuntimePluginAlias() {
	if p == nil || p.rt == nil {
		return
	}
	if err := p.rt.RegisterSharedResource(sharedPluginResourceName, p); err != nil {
		log.Warnf("failed to register Swagger shared plugin alias: %v", err)
	}
}

func (p *PlugSwagger) publishRuntimeContract(ready, healthy bool) {
	if p == nil || p.rt == nil {
		return
	}
	for _, item := range []struct {
		name  string
		value any
	}{
		{name: sharedReadinessResourceName, value: ready},
		{name: sharedHealthResourceName, value: healthy},
	} {
		if err := p.rt.RegisterSharedResource(item.name, item.value); err != nil {
			log.Warnf("failed to register Swagger shared runtime contract %s: %v", item.name, err)
		}
	}
	if err := p.rt.RegisterPrivateResource(privateReadinessResourceName, ready); err != nil {
		log.Warnf("failed to register Swagger private readiness resource: %v", err)
	}
	if err := p.rt.RegisterPrivateResource(privateHealthResourceName, healthy); err != nil {
		log.Warnf("failed to register Swagger private health resource: %v", err)
	}
}
