package zp

type Version interface {
	UnMarshall(message []byte) Message
	Marshall(Message) []byte
	NewReq(body []byte, req uint16, cha []byte) Message
}
