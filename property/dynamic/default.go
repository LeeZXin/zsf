package dynamic

const (
	DefaultType = "default"
)

var (
	defaultImpl = newDefaultProperty()
)

type defaultProperty struct {
}

func newDefaultProperty() *defaultProperty {
	return new(defaultProperty)
}

func (p *defaultProperty) GetPropertyType() string {
	return DefaultType
}

func (p *defaultProperty) OnKeyChange(_ string, _ KeyChangeCallback) {
}

func (p *defaultProperty) OnApplicationStart() {
}

func (p *defaultProperty) AfterInitialize() {
}

func (p *defaultProperty) OnApplicationShutdown() {
}
