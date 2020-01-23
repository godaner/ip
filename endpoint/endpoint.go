package endpoint

type Endpoint interface {
	Start() error
	Stop() error
	Restart() error
	IsStart() bool
	Destroy() error
	GetID() uint16
}
