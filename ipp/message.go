package ipp

const (
	_ = iota
	// v1 简单的交互，不支持加密等
	VERSION_V1 = iota
	// v2 支持aes加密
	VERSION_V2 = iota
)

const (
	_ = iota
	// 业务数据请求交互
	MSG_TYPE_REQ = iota
	// client发起hello，交换初始数据，如client_wanna_proxy_port等；
	MSG_TYPE_CLIENT_HELLO = iota
	// proxy发起hello，交换或者确认初始数据，如client_wanna_proxy_port,client id等；
	MSG_TYPE_PROXY_HELLO = iota
	// 连接建立通知
	MSG_TYPE_CONN_CREATE = iota
	// 已收到连接建立通知
	MSG_TYPE_CONN_CREATE_DONE = iota
	// 连接断开通知
	MSG_TYPE_CONN_CLOSE = iota
	// 心跳
	MSG_TYPE_CONN_HB = iota
)

const (
	_ = iota
	// 业务数据
	ATTR_TYPE_BODY = iota
	// 端口数据
	ATTR_TYPE_PORT = iota
	// client id
	ATTR_TYPE_CLI_ID = iota
)

const (
	_ = iota
	// brwoser端口被占用
	ERROR_CODE_BROWSER_PORT_OCUP = iota
	// 版本不匹配
	ERROR_CODE_VERSION_NOT_MATCH = iota
)

type Message interface {
	UnMarshall(message []byte) (err error)
	Marshall() []byte
	Type() byte
	CID() uint16
	SerialId() uint16
	CliID() uint16
	ErrorCode() byte
	Version() byte
	Attribute(int) Attr
	AttributeByType(byte) []byte
	ForClientHelloReq(port []byte, sID uint16)
	ForServerHelloReq(cliID []byte, port []byte, sID uint16, errCode byte)
	ForReq(body []byte, cliID, cID, sID uint16)
	ForConnCreate(body []byte, cliID, cID, sID uint16)
	ForConnClose(body []byte, cliID, cID, sID uint16)
	ForConnCreateDone(body []byte, cliID, cID, sID uint16)
	ForConnHB(cliID, cID, sID uint16)
}
