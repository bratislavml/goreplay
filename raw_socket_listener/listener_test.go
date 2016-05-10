package rawSocket

import (
	"bytes"
	"log"
	"testing"
	"time"
	"math/rand"
	"sync/atomic"
)

func TestRawListenerInput(t *testing.T) {
	var req, resp *TCPMessage

	listener := NewListener("", "0", EnginePcap, 10*time.Millisecond)
	defer listener.Close()

	reqPacket := buildPacket(true, 1, 1, []byte("GET / HTTP/1.1\r\n\r\n"))

	respAck := reqPacket.Seq + uint32(len(reqPacket.Data))
	respPacket := buildPacket(false, respAck, reqPacket.Seq+1, []byte("HTTP/1.1 200 OK\r\n\r\n"))

	listener.processTCPPacket(reqPacket)
	listener.processTCPPacket(respPacket)

	select {
	case req = <-listener.messagesChan:
	case <-time.After(time.Millisecond):
		t.Error("Should return request immediately")
		return
	}

	if !req.IsIncoming {
		t.Error("Should be request")
	}

	select {
	case resp = <-listener.messagesChan:
	case <-time.After(20 * time.Millisecond):
		t.Error("Should return response immediately")
		return
	}

	if resp.IsIncoming {
		t.Error("Should be response")
	}
}

func TestRawListenerResponse(t *testing.T) {
	var req, resp *TCPMessage

	listener := NewListener("", "0", EnginePcap, 10*time.Millisecond)
	defer listener.Close()

	reqPacket := buildPacket(true, 1, 1, []byte("GET / HTTP/1.1\r\n\r\n"))
	respPacket := buildPacket(false, 1+uint32(len(reqPacket.Data)), 2, []byte("HTTP/1.1 200 OK\r\n\r\n"))

	// If response packet comes before request
	listener.processTCPPacket(respPacket)
	listener.processTCPPacket(reqPacket)

	select {
	case req = <-listener.messagesChan:
	case <-time.After(time.Millisecond):
		t.Error("Should return respose immediately")
		return
	}

	if !req.IsIncoming {
		t.Error("Should be request")
	}

	select {
	case resp = <-listener.messagesChan:
	case <-time.After(time.Millisecond):
		t.Error("Should return response immediately")
		return
	}

	if resp.IsIncoming {
		t.Error("Should be response")
	}

	if !bytes.Equal(resp.UUID(), req.UUID()) {
		t.Error("Resp and Req UUID should be equal")
	}
}

func TestRawListener100Continue(t *testing.T) {
	var req, resp *TCPMessage

	listener := NewListener("", "0", EnginePcap, 10*time.Millisecond)
	defer listener.Close()

	reqPacket1 := buildPacket(true, 1, 1, []byte("POST / HTTP/1.1\r\nContent-Length: 2\r\nExpect: 100-continue\r\n\r\n"))
	// Packet with data have different Seq
	reqPacket2 := buildPacket(true, 2, reqPacket1.Seq+uint32(len(reqPacket1.Data)), []byte("a"))
	reqPacket3 := buildPacket(true, 2, reqPacket2.Seq+1, []byte("b"))

	respPacket1 := buildPacket(false, 10, 3, []byte("HTTP/1.1 100 Continue\r\n"))

	// panic(int(uint32(len(reqPacket1.Data)) + uint32(len(reqPacket2.Data)) + uint32(len(reqPacket3.Data))))
	respPacket2 := buildPacket(false, reqPacket3.Seq+1 /* len of data */, 2, []byte("HTTP/1.1 200 OK\r\n"))

	listener.processTCPPacket(reqPacket1)
	listener.processTCPPacket(reqPacket2)
	listener.processTCPPacket(reqPacket3)

	listener.processTCPPacket(respPacket1)
	listener.processTCPPacket(respPacket2)

	select {
	case req = <-listener.messagesChan:
		break
	case <-time.After(11 * time.Millisecond):
		t.Error("Should return request after expire time")
		return
	}

	if !bytes.Equal(req.Bytes(), []byte("POST / HTTP/1.1\r\nContent-Length: 2\r\n\r\nab")) {
		t.Error("Should receive full message", string(req.Bytes()))
	}

	if !req.IsIncoming {
		t.Error("Should be request")
	}

	select {
	case resp = <-listener.messagesChan:
		break
	case <-time.After(21 * time.Millisecond):
		t.Error("Should return response after expire time")
		return
	}

	if resp.IsIncoming {
		t.Error("Should be response")
	}

	if !bytes.Equal(resp.UUID(), req.UUID()) {
		t.Error("Resp and Req UUID should be equal")
	}
}

// Response comes before Request
func TestRawListener100ContinueWrongOrder(t *testing.T) {
	var req, resp *TCPMessage

	listener := NewListener("", "0", EnginePcap, 10*time.Millisecond)
	defer listener.Close()

	reqPacket1 := buildPacket(true, 1, 1, []byte("POST / HTTP/1.1\r\nContent-Length: 2\r\nExpect: 100-continue\r\n\r\n"))
	// Packet with data have different Seq
	reqPacket2 := buildPacket(true, 2, reqPacket1.Seq+uint32(len(reqPacket1.Data)), []byte("a"))
	reqPacket3 := buildPacket(true, 2, reqPacket2.Seq+1, []byte("b"))

	respPacket1 := buildPacket(false, 10, 3, []byte("HTTP/1.1 100 Continue\r\n"))

	// panic(int(uint32(len(reqPacket1.Data)) + uint32(len(reqPacket2.Data)) + uint32(len(reqPacket3.Data))))
	respPacket2 := buildPacket(false, reqPacket3.Seq+1 /* len of data */, 2, []byte("HTTP/1.1 200 OK\r\n"))

	listener.processTCPPacket(respPacket1)
	listener.processTCPPacket(respPacket2)

	listener.processTCPPacket(reqPacket1)
	listener.processTCPPacket(reqPacket2)
	listener.processTCPPacket(reqPacket3)

	select {
	case req = <-listener.messagesChan:
		break
	case <-time.After(11 * time.Millisecond):
		t.Error("Should return response after expire time")
		return
	}

	if !bytes.Equal(req.Bytes(), []byte("POST / HTTP/1.1\r\nContent-Length: 2\r\n\r\nab")) {
		t.Error("Should receive full message", string(req.Bytes()))
	}

	if !req.IsIncoming {
		t.Error("Should be request")
	}

	select {
	case resp = <-listener.messagesChan:
		break
	case <-time.After(21 * time.Millisecond):
		t.Error("Should return response after expire time")
		return
	}

	if resp.IsIncoming {
		t.Error("Should be response")
	}

	if !bytes.Equal(resp.UUID(), req.UUID()) {
		t.Error("Resp and Req UUID should be equal")
	}
}

func testChunkedSequence(t *testing.T, listener *Listener, packets ...*TCPPacket) {
	var r, req, resp *TCPMessage

	for _, p := range packets {
		listener.processTCPPacket(p)
	}

	select {
	case r = <-listener.messagesChan:
		if r.IsIncoming {
			req = r
		} else {
			resp = r
		}
		break
	case <-time.After(25 * time.Millisecond):
		t.Error("Should return request after expire time")
		return
	}
	select {
	case r = <-listener.messagesChan:
		if r.IsIncoming {
			if req != nil {
				t.Error("Request already received", r)
				return
			}
			req = r
		} else {
			if resp != nil {
				t.Error("Response already received", r)
				return
			}
			resp = r
		}
		break
	case <-time.After(25 * time.Millisecond):
		t.Error("Should return request after expire time")
		return
	}

	if !bytes.Equal(req.Bytes(), []byte("POST / HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n1\r\na\r\n1\r\nb\r\n0\r\n\r\n")) {
		t.Error("Should receive full message", string(req.Bytes()))
	}

	if !req.IsIncoming {
		t.Error("Should be request")
	}

	if resp.IsIncoming {
		t.Error("Should be response")
	}

	if !bytes.Equal(resp.UUID(), req.UUID()) {
		t.Error("Resp and Req UUID should be equal", string(resp.UUID()), string(req.UUID()))
	}

	time.Sleep(15 * time.Millisecond)

	if len(listener.messages) != 0 {
		t.Error("Messages non empty:", listener.messages)
	}
}

func permutation(n int, list []*TCPPacket) []*TCPPacket {
	if len(list) == 1 {
		return list
	}

	k := n % len(list)

	first := []*TCPPacket{list[k]}
	next := make([]*TCPPacket, len(list)-1)

	copy(next, append(list[:k], list[k+1:]...))

	return append(first, permutation(n/len(list), next)...)
}

// Response comes before Request
func TestRawListenerChunkedWrongOrder(t *testing.T) {
	listener := NewListener("", "0", EnginePcap, 10*time.Millisecond)
	defer listener.Close()

	reqPacket1 := buildPacket(true, 1, 1, []byte("POST / HTTP/1.1\r\nTransfer-Encoding: chunked\r\nExpect: 100-continue\r\n\r\n"))
	// Packet with data have different Seq
	reqPacket2 := buildPacket(true, 2, reqPacket1.Seq+uint32(len(reqPacket1.Data)), []byte("1\r\na\r\n"))
	reqPacket3 := buildPacket(true, 2, reqPacket2.Seq+uint32(len(reqPacket2.Data)), []byte("1\r\nb\r\n"))
	reqPacket4 := buildPacket(true, 2, reqPacket3.Seq+uint32(len(reqPacket3.Data)), []byte("0\r\n\r\n"))

	respPacket1 := buildPacket(false, 10, 3, []byte("HTTP/1.1 100 Continue\r\n"))

	// panic(int(uint32(len(reqPacket1.Data)) + uint32(len(reqPacket2.Data)) + uint32(len(reqPacket3.Data))))
	respPacket2 := buildPacket(false, reqPacket4.Seq+5 /* len of data */, 2, []byte("HTTP/1.1 200 OK\r\n"))

	// Should re-construct message from all possible combinations
	for i := 0; i < 6*5*4*3*2*1; i++ {
		packets := permutation(i, []*TCPPacket{reqPacket1, reqPacket2, reqPacket3, reqPacket4, respPacket1, respPacket2})
		testChunkedSequence(t, listener, packets...)
	}
}


func chunkedPostMessage() []*TCPPacket {
	ack := uint32(rand.Int63())
	seq := uint32(rand.Int63())

	reqPacket1 := buildPacket(true, ack, seq, []byte("POST / HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n"))
	// Packet with data have different Seq
	reqPacket2 := buildPacket(true, ack, seq + 47, []byte("1\r\na\r\n"))
	reqPacket3 := buildPacket(true, ack, reqPacket2.Seq+5, []byte("1\r\nb\r\n"))
	reqPacket4 := buildPacket(true, ack, reqPacket3.Seq+5, []byte("0\r\n\r\n"))

	respPacket := buildPacket(false, reqPacket4.Seq+5 /* len of data */, ack, []byte("HTTP/1.1 200 OK\r\n"))

	return []*TCPPacket{
		reqPacket1, reqPacket2, reqPacket3, reqPacket4, respPacket,
	}
}

func postMessage() []*TCPPacket {
	ack := uint32(rand.Int63())
	seq2 := uint32(rand.Int63())
	seq := uint32(rand.Int63())

	c := 10000
	data := make([]byte, c)
	rand.Read(data)

	head := []byte("POST / HTTP/1.1\r\nContent-Length: 9958\r\n\r\n")
	for i, _ := range head {
		data[i] = head[i]
	}

	return []*TCPPacket{
		buildPacket(true, ack, seq, data),
		buildPacket(false, seq + uint32(len(data)), seq2, []byte("HTTP/1.1 200 OK\r\n")),
	}
}

func getMessage() []*TCPPacket {
	ack := uint32(rand.Int63())
	seq2 := uint32(rand.Int63())
	seq := uint32(rand.Int63())

	return []*TCPPacket{
		buildPacket(true, ack, seq, []byte("GET / HTTP/1.1\r\n\r\n")),
		buildPacket(false, seq + 18, seq2, []byte("HTTP/1.1 200 OK\r\n")),
	}
}

// Response comes before Request
func TestRawListenerBench(t *testing.T) {
	l := NewListener("", "0", EnginePcap, 200*time.Millisecond)
	defer l.Close()

	// Should re-construct message from all possible combinations
	for i := 0; i < 1000; i++ {
		go func(){
			for j := 0; j < 100; j++ {
				var packets []*TCPPacket

				if j % 5 == 0 {
					packets = chunkedPostMessage()
				} else if j % 3 == 0 {
				 	packets = postMessage()
				} else {
					packets = getMessage()
				}

				for _, p := range packets {
					// Randomly drop packets
					if (i + j) % 5 == 0 {
						if rand.Int63() % 3 == 0 {
							continue
						}
					}

					l.packetsChan <- p.Dump()
					time.Sleep(time.Millisecond)
				}

				time.Sleep(5 * time.Millisecond)
			}
		}()
	}

	ch := l.Receiver()

	var count int32

	for {
		select {
			case <- ch:
				atomic.AddInt32(&count, 1)
			case <-time.After(2000 * time.Millisecond):
				log.Println("Emitted 200000 messages, captured: ", count, len(l.ackAliases), len(l.seqWithData), len(l.respAliases), len(l.respWithoutReq), len(l.packetsChan))
				return
		}
	}
}