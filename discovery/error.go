package discovery

type ErrorDiscovery struct {
	discoveryType string
	err           error
}

func (d *ErrorDiscovery) GetDiscoveryType() string {
	return d.discoveryType
}

func (d *ErrorDiscovery) GetServiceInfo(name string) ([]ServiceAddr, error) {
	return nil, d.err
}
