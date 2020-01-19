package v1

import (
	"encoding/binary"
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"net"
	"testing"
	"time"
)

func TestVersion_NewReq(t *testing.T) {
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(1025))
	ippLength := binary.BigEndian.Uint32(ippLen)
	fmt.Println("ippLength",ippLength)
	fmt.Println(ippLength)
	Convey("New Req and UnMarshall !", t, func(cc C) {
		testBody := []byte("my love")
		t.Logf("origin is : %v !",testBody)
		ri := uint16(123)
		m1 := new(Message)
		m1.ForReq(testBody, ri)
		So(m1, ShouldNotBeNil)

		m2 := new(Message)
		b:=m1.Marshall()
		t.Logf("after ma is : %v !",b)
		m2.UnMarshall(b)
		t.Logf("after uma is : %v !",m2.Marshall())
		So(m2, ShouldNotBeNil)
		So(m2.CID(), ShouldEqual, ri)
		So(len(m2.Marshall()), ShouldBeGreaterThan, 0)
		t.Logf("bytes is : %v !", m2.Marshall())

	})

}
func TestMessage_Marshall(t *testing.T) {
	Convey("TCP with IPP !", t, func(cc C) {
		testBody := []byte("my love")
		go func(cc C) {
			l, err := net.Listen("tcp", ":1111")
			cc.So(err, ShouldBeNil)
			for {
				conn, err := l.Accept()
				go func(cc C) {
					for {
						fmt.Println("2")
						cc.So(err, ShouldBeNil)
						bs := make([]byte, 40920, 40920)
						n, err := conn.Read(bs)
						cc.So(err, ShouldBeNil)
						s := string(bs[0:n]) + "1"
						n, err = conn.Write([]byte(s))
						cc.So(err, ShouldBeNil)
						cc.So(n, ShouldBeGreaterThan, 0)
					}
				}(cc)
			}

		}(cc)
		conn, err := net.Dial("tcp", ":1111")
		cc.So(err, ShouldBeNil)
		go func(cc C) {
			for {
				bs := make([]byte, 40920, 40920)
				n, err := conn.Read(bs)
				fmt.Println("3")
				cc.So(err, ShouldBeNil)
				cc.So(n, ShouldBeGreaterThan, 0)
				fmt.Printf("info is : %v !", string(bs[0:n]))

			}
		}(cc)

		for {
			n, err := conn.Write(testBody)
			fmt.Println("1")
			cc.So(err, ShouldBeNil)
			cc.So(n, ShouldBeGreaterThan, 0)
			time.Sleep(time.Duration(1) * time.Second)
		}
	})
}
