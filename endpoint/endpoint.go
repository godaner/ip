package endpoint

type Status string
type Event string

const (
	Status_Stoped    = Status("stoped")
	Status_Started   = Status("started")
	Status_Destroied = Status("destroied")
)
const (
	Event_Stop    = Event("stop")
	Event_Start   = Event("start")
	Event_Destroy = Event("destroy")
)
type Endpoint interface {
	Start() error
	Stop() error
	Restart() error
	Status() Status
	Destroy() error
	GetID() uint16
}
