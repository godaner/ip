package v1

import (
	"bytes"
	"encoding/binary"
	"github.com/godaner/ip/ipp"
	"log"
)

// Header
type Header struct {
	HVersion byte
	HType    byte
	HCID     uint16
	HAttrNum byte
}

func (h *Header) Version() byte {
	return h.HVersion
}

func (h *Header) Type() byte {
	return h.HType
}

func (h *Header) CID() uint16 {
	return h.HCID
}

func (h *Header) AttrNum() byte {
	return h.HAttrNum
}

// Attr
type Attr struct {
	AT byte
	AL uint32
	AV []byte
}

func (a *Attr) T() byte {
	return a.AT
}

func (a *Attr) L() uint32 {
	return a.AL
}

func (a *Attr) V() []byte {
	return a.AV
}

// Message
type Message struct {
	Header   Header
	Attr     []Attr
	AttrMaps map[byte][]byte
}


func (m *Message) AttributeByType(t byte) []byte {
	return m.AttrMaps[t]
}

func (m *Message) Type() byte {
	return m.Header.Type()
}

func (m *Message) CID() uint16 {
	return m.Header.CID()
}
func (m *Message) Attribute(index int) ipp.Attr {
	return &m.Attr[index]
}
func (m *Message) Marshall() []byte {
	buf := new(bytes.Buffer)
	var err error
	err = binary.Write(buf, binary.BigEndian, m.Header.HVersion)
	if err != nil {
		log.Printf("Message#Bytes : binary.Write m.Header.Version err , err is : %v !", err.Error())
	}
	err = binary.Write(buf, binary.BigEndian, m.Header.HType)
	if err != nil {
		log.Printf("Message#Bytes : binary.Write m.Header.Type err , err is : %v !", err.Error())
	}
	err = binary.Write(buf, binary.BigEndian, m.Header.HCID)
	if err != nil {
		log.Printf("Message#Bytes : binary.Write m.Header.HCID err , err is : %v !", err.Error())
	}
	err = binary.Write(buf, binary.BigEndian, m.Header.HAttrNum)
	if err != nil {
		log.Printf("Message#Bytes : binary.Write m.Header.AttrNum err , err is : %v !", err.Error())
	}
	for _, v := range m.Attr {
		err = binary.Write(buf, binary.BigEndian, v.AT)
		if err != nil {
			log.Printf("Message#Bytes : binary.Write m.Header.AttrType err , err is : %v !", err.Error())
		}
		err = binary.Write(buf, binary.BigEndian, v.AL+2)
		if err != nil {
			log.Printf("Message#Bytes : binary.Write m.Header.AttrLen err , err is : %v !", err.Error())
		}
		err = binary.Write(buf, binary.BigEndian, v.AV)
		if err != nil {
			log.Printf("Message#Bytes : binary.Write m.Header.AttrStr err , err is : %v !", err.Error())
		}
	}
	body := buf.Bytes()
	// be careful !!
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(body)))
	body = append(ippLen, body...)
	return body
}

func (m *Message) UnMarshall(message []byte) {
	buf := bytes.NewBuffer(message)
	if err := binary.Read(buf, binary.BigEndian, &m.Header.HVersion); err != nil {
		log.Printf("Message#UnMarshall : binary.Readm.Header.HVersion err , err is : %v !", err.Error())
	}
	if err := binary.Read(buf, binary.BigEndian, &m.Header.HType); err != nil {
		log.Printf("Message#UnMarshall : binary.Readm.Header.HType err , err is : %v !", err.Error())
	}
	if err := binary.Read(buf, binary.BigEndian, &m.Header.HCID); err != nil {
		log.Printf("Message#UnMarshall : binary.Readm.Header.HCID err , err is : %v !", err.Error())
	}
	if err := binary.Read(buf, binary.BigEndian, &m.Header.HAttrNum); err != nil {
		log.Printf("Message#UnMarshall : binary.Readm.Header.HAttrNum err , err is : %v !", err.Error())
	}

	m.Attr = make([]Attr, m.Header.AttrNum())
	m.AttrMaps = make(map[byte][]byte)
	for i := byte(0); i < m.Header.AttrNum(); i++ {
		attr := &m.Attr[i]
		err := binary.Read(buf, binary.BigEndian, &attr.AT)
		if err != nil {
			log.Printf("Message#UnMarshall : binary.Read 0 err , err is : %v !", err.Error())
		}
		err = binary.Read(buf, binary.BigEndian, &attr.AL)
		if err != nil {
			log.Printf("Message#UnMarshall : binary.Read 1 err , err is : %v !", err.Error())
		}
		attr.AL -= 2
		attr.AV = make([]byte, attr.L())
		if err := binary.Read(buf, binary.BigEndian, &attr.AV); err != nil {
			log.Printf("Message#UnMarshall : binary.Read 2 err , err is : %v !", err.Error())
		}
		m.AttrMaps[attr.AT] = attr.AV

	}
}

func (m *Message) ForConnCreate(body []byte, cID uint16) {
	m.newMessage(ipp.MSG_TYPE_CONN_CREATE, cID)
}

func (m *Message) ForConnClose(body []byte, cID uint16) {
	m.newMessage(ipp.MSG_TYPE_CONN_CLOSE, cID)
}
func (m *Message) ForHelloReq(body []byte, cID uint16) {
	m.newMessage(ipp.MSG_TYPE_HELLO, cID)
	m.Attr = []Attr{
		{
			AT: ipp.ATTR_TYPE_PORT, AL: uint32(len(body)), AV: body,
		},
	}
	m.Header.HAttrNum = byte(len(m.Attr))
}
func (m *Message) ForReq(body []byte, cID uint16) {
	m.newMessage(ipp.MSG_TYPE_REQ, cID)
	m.Attr = []Attr{
		{
			AT: ipp.ATTR_TYPE_BODY, AL: uint32(len(body)), AV: body,
		},
	}
	m.Header.HAttrNum = byte(len(m.Attr))
}

func (m *Message) newMessage(typ byte, cID uint16) {
	header := Header{}
	header.HVersion = ipp.VERSION_V1
	header.HType = typ
	header.HCID = cID
	m.Header = header
}
