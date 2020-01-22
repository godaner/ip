package progress
// Progress
//  such as console , ui etc.
type Progress interface {
	Launch() (err error)
}
