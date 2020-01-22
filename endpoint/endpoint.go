package endpoint

type Endpoint interface {
	Start() error
	Stop() error
}
