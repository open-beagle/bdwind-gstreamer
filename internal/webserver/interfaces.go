package webserver

import (
	"context"

	"github.com/gorilla/mux"
)

// RouteSetup 路由设置接口
// 所有需要注册HTTP路由的组件都应该实现此接口
type RouteSetup interface {
	// SetupRoutes 设置组件的HTTP路由
	// router: 用于注册路由的mux路由器
	// 返回错误如果路由设置失败
	SetupRoutes(router *mux.Router) error
}

// ComponentManager 组件管理器接口
// 定义了组件的完整生命周期管理和路由集成能力
type ComponentManager interface {
	// 继承路由设置能力
	RouteSetup

	// Start 启动组件
	// ctx: 用于控制启动超时和生命周期的上下文
	// 返回错误如果启动失败
	Start(ctx context.Context) error

	// Stop 停止组件
	// ctx: 用于控制停止超时的上下文
	// 返回错误如果停止失败
	Stop(ctx context.Context) error

	// IsEnabled 检查组件是否启用
	// 返回true如果组件在配置中被启用，否则返回false
	IsEnabled() bool

	// IsRunning 检查组件是否正在运行
	// 返回true如果组件正在运行，否则返回false
	IsRunning() bool

	// GetStats 获取组件的统计信息
	// 返回包含组件状态和统计数据的map
	GetStats() map[string]interface{}

	// GetContext 获取组件的上下文
	// 返回组件当前使用的上下文，如果组件未启动则返回nil
	GetContext() context.Context
}

// ComponentRegistry 组件注册表接口
// 用于管理已注册的组件
type ComponentRegistry interface {
	// RegisterComponent 注册一个组件
	// name: 组件名称，用于标识组件
	// component: 实现ComponentManager接口的组件实例
	// 返回错误如果注册失败（如名称冲突）
	RegisterComponent(name string, component ComponentManager) error

	// UnregisterComponent 注销一个组件
	// name: 要注销的组件名称
	// 返回错误如果组件不存在
	UnregisterComponent(name string) error

	// GetComponent 获取已注册的组件
	// name: 组件名称
	// 返回组件实例和是否存在的标志
	GetComponent(name string) (ComponentManager, bool)

	// ListComponents 列出所有已注册的组件名称
	// 返回组件名称列表
	ListComponents() []string
}

// MiddlewareProvider 中间件提供者接口
// 组件可以实现此接口来提供HTTP中间件
type MiddlewareProvider interface {
	// GetMiddlewares 获取组件提供的中间件列表
	// 返回中间件函数切片，按执行顺序排列
	GetMiddlewares() []mux.MiddlewareFunc
}

// HealthChecker 健康检查接口
// 组件可以实现此接口来提供健康检查功能
type HealthChecker interface {
	// HealthCheck 执行健康检查
	// 返回健康状态信息和错误（如果不健康）
	HealthCheck() (map[string]interface{}, error)
}
