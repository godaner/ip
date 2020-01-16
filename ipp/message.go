package ipp

const (
	_ = iota
	// v1 简单的交互，不支持加密等
	VERSION_V1 = iota
)

const (
	_ = iota
	// 业务数据请求交互
	MSG_TYPE_REQ = iota
	// 初始数据交换，如client_wanna_proxy_port；
	MSG_TYPE_HELLO = iota
	// proxy和browser的连接建立
	MSG_TYPE_CONN_CREATE = iota
	// proxy和browser的连接断开
	MSG_TYPE_CONN_CLOSE = iota
)

const (
	_ = iota
	// 业务数据
	ATTR_TYPE_BODY = iota
	// 端口数据
	ATTR_TYPE_PORT = iota
)

type Message interface {
	UnMarshall(message []byte)
	Marshall() []byte
	Type() byte
	ReqId() uint16
	SerialId() uint16
	Attribute(int) Attr
	AttributeByType(byte) []byte
	ForReq(body []byte, req uint16)
	ForHelloReq(body []byte, req uint16)
	ForConnCreate(body []byte, req uint16)
	ForConnClose(body []byte, req uint16)
}
