package zp

type Message interface {
	UnMarshall(message []byte)
	Marshall() []byte
	NewReq(body []byte, req uint16)
	Type() byte
	ReqId() uint16
	SerialId() uint16
}
