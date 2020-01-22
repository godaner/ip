package progress

type Progress interface {
	Start() (err error)
	Stop() (err error)
}
