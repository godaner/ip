package endpoint

type Endpoint interface {
	Start() error
	Restart() error
	IsStart() bool
	Stop() error
	GetID() (id uint16)
}
