package prometheus

import "strings"

// Factory 使用统一命名空间和子系统创建指标。
type Factory struct {
	registry  *Registry
	namespace string
	subsystem string
}

// NewFactory 创建指标工厂；工厂不持有 goroutine 或其他生命周期资源。
func NewFactory(registry *Registry, namespace, subsystem string) (*Factory, error) {
	if registry == nil {
		return nil, ErrNilRegistry
	}
	return &Factory{registry: registry, namespace: namespace, subsystem: subsystem}, nil
}

// Counter 获取或注册计数器。
func (f *Factory) Counter(name, help string, labels ...string) (*Counter, error) {
	return f.registry.Counter(f.fullName(name), help, labels...)
}

// Gauge 获取或注册仪表。
func (f *Factory) Gauge(name, help string, labels ...string) (*Gauge, error) {
	return f.registry.Gauge(f.fullName(name), help, labels...)
}

// Histogram 获取或注册直方图。
func (f *Factory) Histogram(name, help string, buckets []float64, labels ...string) (*Histogram, error) {
	return f.registry.Histogram(f.fullName(name), help, buckets, labels...)
}

// Summary 获取或注册摘要指标。
func (f *Factory) Summary(name, help string, quantiles map[float64]float64, labels ...string) (*Summary, error) {
	return f.registry.Summary(f.fullName(name), help, quantiles, labels...)
}

func (f *Factory) fullName(name string) string {
	return metricName(f.namespace, f.subsystem, name)
}

func metricName(namespace, subsystem, name string) string {
	parts := make([]string, 0, 3)
	if namespace != "" {
		parts = append(parts, namespace)
	}
	if subsystem != "" {
		parts = append(parts, subsystem)
	}
	parts = append(parts, name)
	return strings.Join(parts, "_")
}
