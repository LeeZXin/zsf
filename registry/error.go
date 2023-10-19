package registry

type ErrorRegistry struct {
	registryType string
	err          error
}

func (r *ErrorRegistry) GetRegistryType() string {
	return r.registryType
}

func (r *ErrorRegistry) StartRegisterSelf(ServiceInfo) error {
	return r.err
}
