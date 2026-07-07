package mtprotoedge

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/gotd/td/mtproxy/obfuscated2"
	"github.com/gotd/td/proto/codec"

	"telesrv/internal/clientaddr"
)

func newWebsocketListener(addr net.Addr) (net.Listener, http.Handler) {
	l := &websocketListener{
		addr:   addr,
		ch:     make(chan *websocketServerConn, 1),
		closed: make(chan struct{}),
	}
	return l, l
}

type websocketListener struct {
	addr   net.Addr
	ch     chan *websocketServerConn
	closed chan struct{}
	once   sync.Once
}

func (l *websocketListener) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	wsConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{"binary"},
	})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer func() {
		_ = wsConn.Close(websocket.StatusNormalClosure, "Close")
	}()

	info := clientaddr.FromHTTP(r)
	conn := &websocketNetConn{
		Conn: websocket.NetConn(context.Background(), wsConn, websocket.MessageBinary),
		info: info,
	}
	rw, md, err := obfuscated2.Accept(conn, nil)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var tag *bytes.Reader
	if md.Protocol[0] == codec.AbridgedClientStart[0] {
		tag = bytes.NewReader(md.Protocol[:1])
	} else {
		tag = bytes.NewReader(md.Protocol[:])
	}

	accepted := &websocketServerConn{
		Conn:   conn,
		reader: io.MultiReader(tag, rw),
		writer: rw,
		closed: make(chan struct{}),
		info:   info,
	}

	select {
	case <-r.Context().Done():
		return
	case <-l.closed:
		return
	case l.ch <- accepted:
	}

	select {
	case <-r.Context().Done():
	case <-l.closed:
	case <-accepted.closed:
	}
}

func (l *websocketListener) Accept() (net.Conn, error) {
	select {
	case <-l.closed:
		return nil, net.ErrClosed
	case conn := <-l.ch:
		return conn, nil
	}
}

func (l *websocketListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *websocketListener) Addr() net.Addr {
	return l.addr
}

type websocketNetConn struct {
	net.Conn
	info clientaddr.Info
}

func (c *websocketNetConn) ClientAddressInfo() clientaddr.Info {
	return c.info
}

type websocketServerConn struct {
	net.Conn
	reader io.Reader
	writer io.Writer
	closed chan struct{}
	once   sync.Once
	info   clientaddr.Info
}

func (c *websocketServerConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func (c *websocketServerConn) Write(p []byte) (int, error) {
	return c.writer.Write(p)
}

func (c *websocketServerConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func (c *websocketServerConn) ClientAddressInfo() clientaddr.Info {
	return c.info
}
