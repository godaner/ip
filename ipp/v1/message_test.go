package v1

import (
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"net"
	"testing"
	"time"
)

func TestVersion_NewReq(t *testing.T) {

	Convey("New Req and UnMarshall !", t, func(cc C) {
		testBody := []byte("my love")
		ri := uint16(123)
		m1 := new(Message)
		m1.ForReq(testBody, ri)
		So(m1, ShouldNotBeNil)

		m2 := new(Message)
		m2.UnMarshall(m1.Marshall())
		So(m2, ShouldNotBeNil)
		So(m2.SerialId(), ShouldBeGreaterThan, 0)
		So(m2.ReqId(), ShouldEqual, ri)
		So(len(m2.Marshall()), ShouldBeGreaterThan, 0)
		t.Logf("bytes is : %v !", m2.Marshall())

	})
	Convey("TCP with IPP !", t, func(cc C) {
		testBody := []byte("my love")
		go func(cc C) {
			l,err:=net.Listen("tcp",":1111")
			cc.So(err,ShouldBeNil)
			for ; ;  {
				conn,err:=l.Accept()
				for ; ; {
					fmt.Println("2")
					cc.So(err,ShouldBeNil)
					bs:=make([]byte,1024,1024)
					n,err:=conn.Read(bs)
					cc.So(err,ShouldBeNil)
					s:=string(bs[0:n])+"1"
					n,err=conn.Write([]byte(s))
					cc.So(err,ShouldBeNil)
					cc.So(n,ShouldBeGreaterThan,0)
				}
			}

		}(cc)
		conn,err:=net.Dial("tcp",":1111")
		cc.So(err,ShouldBeNil)
		go func(cc C) {
			for ; ; {
				bs:=make([]byte,1024,1024)
				n,err:=conn.Read(bs)
				fmt.Println("3")
				cc.So(err,ShouldBeNil)
				cc.So(n,ShouldBeGreaterThan,0)
				fmt.Printf("info is : %v !",string(bs[0:n]))

			}
		}(cc)

		for ; ;  {
			n,err:=conn.Write(testBody)
			fmt.Println("1")
			cc.So(err,ShouldBeNil)
			cc.So(n,ShouldBeGreaterThan,0)
			time.Sleep(time.Duration(1)*time.Second)
		}
	})
}
