# WebServer Component Registration Architecture

## Overview

The WebServer has been refactored to implement a component registration mechanism that allows for modular, loosely-coupled architecture. Instead of directly depending on specific business components, the WebServer now provides a registration system where components can register themselves and their routes.

## Key Features

### 1. Component Registration
- `RegisterComponent(name string, component ComponentManager) error` - Register a component
- `UnregisterComponent(name string) error` - Unregister a component  
- `GetComponent(name string) (ComponentManager, bool)` - Retrieve a registered component
- `ListComponents() []string` - List all registered component names

### 2. Simplified Constructor
The WebServer constructor now only requires basic configuration:
```go
func NewWebServer(cfg *config.WebServerConfig) (*WebServer, error)
```

### 3. Automatic Route Integration
Components that implement the `ComponentManager` interface automatically have their routes integrated when registered:
```go
type ComponentManager interface {
    RouteSetup
    Start() error
    Stop(ctx context.Context) error
    IsRunning() bool
    GetStats() map[string]interface{}
}

type RouteSetup interface {
    SetupRoutes(router *mux.Router) error
}
```

### 4. Preserved Core Functionality
- Middleware support (CORS, logging)
- Static file serving
- Basic API endpoints (/api/status, /api/version, /health)
- TLS support

## Usage Example

```go
// Create WebServer
cfg := &config.WebServerConfig{
    Host: "localhost",
    Port: 8080,
    EnableCORS: true,
}
webServer, err := webserver.NewWebServer(cfg)
if err != nil {
    return err
}

// Register components
webrtcMgr := webrtc.NewManager(...)
gstreamerMgr := gstreamer.NewManager(...)
authMgr := auth.NewManager(...)

webServer.RegisterComponent("webrtc", webrtcMgr)
webServer.RegisterComponent("gstreamer", gstreamerMgr)
webServer.RegisterComponent("auth", authMgr)

// Start the server
err = webServer.Start()
```

## Component Implementation

Components must implement the `ComponentManager` interface:

```go
type MyComponent struct {
    // component fields
}

func (c *MyComponent) SetupRoutes(router *mux.Router) error {
    router.HandleFunc("/api/mycomponent/endpoint", c.handleEndpoint).Methods("GET")
    return nil
}

func (c *MyComponent) Start() error {
    // component startup logic
    return nil
}

func (c *MyComponent) Stop(ctx context.Context) error {
    // component shutdown logic
    return nil
}

func (c *MyComponent) IsRunning() bool {
    // return component running status
    return true
}

func (c *MyComponent) GetStats() map[string]interface{} {
    // return component statistics
    return map[string]interface{}{
        "status": "running",
    }
}
```

## Benefits

1. **Loose Coupling**: WebServer no longer depends on specific business components
2. **High Cohesion**: Each component manages its own routes and handlers
3. **Modularity**: Components can be easily added, removed, or replaced
4. **Testability**: Components and WebServer can be tested independently
5. **Maintainability**: Clear separation of concerns

## Migration Notes

- The WebServer constructor signature has changed
- Components must now implement the `ComponentManager` interface
- Route registration is now handled automatically through component registration
- The main application needs to be updated to use the new registration pattern (handled in task 7)