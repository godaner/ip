package zp

type Header interface {
	Version() byte
	Type() byte
	SerialNo() uint16
	ReqIdentifier() uint16
	AttrNum() byte
}
