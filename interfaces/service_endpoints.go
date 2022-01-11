package interfaces

// ServiceEndpoints allow configuration of custom service URIs.
//
// If you want to set non-default values for any of these fields, set the ServiceEndpoints field
// in the SDK's Config struct. You may set individual values such as Streaming, or use the
// helper method ldcomponents.RelayProxyEndpoints().
//
// See Config.ServiceEndpoints for more details.
type ServiceEndpoints struct {
	Streaming string
	Polling   string
	Events    string
}
