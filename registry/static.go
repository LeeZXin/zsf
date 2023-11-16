package registry

type staticRegistry struct{}

func (s *staticRegistry) GetRegistryType() string {
	return StaticRegistryType
}

func (s *staticRegistry) RegisterSelf(_ ServiceInfo) DeregisterAction {
	return &deregisterActionImpl{}
}
