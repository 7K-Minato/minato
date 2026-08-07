package rcon

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
)

// fakeRCONServer implements enough of the Source RCON protocol to verify the
// client sends login (type 3) and command (type 2) packets correctly.
func fakeRCONServer(t *testing.T, password string) (addr string, commands chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	commands = make(chan string, 8)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			var length int32
			if err := binary.Read(conn, binary.LittleEndian, &length); err != nil {
				return
			}
			data := make([]byte, length)
			if _, err := io.ReadFull(conn, data); err != nil {
				return
			}
			ptype := int32(binary.LittleEndian.Uint32(data[4:8]))
			body := data[8:]
			if i := len(body) - 2; i >= 0 {
				body = body[:i]
			}
			switch ptype {
			case 3: // login
				respType := int32(2)
				respID := int32(1)
				if string(body) != password {
					respID = -1
				}
				writeTestPacket(conn, respID, respType, "")
			case 2: // command
				commands <- string(body)
				writeTestPacket(conn, 1, 0, "ok")
			default:
				writeTestPacket(conn, -1, 2, "bad type")
			}
		}
	}()
	return ln.Addr().String(), commands
}

func writeTestPacket(w io.Writer, id, ptype int32, body string) {
	length := int32(4 + 4 + len(body) + 2)
	buf := make([]byte, 4+length)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(length))
	binary.LittleEndian.PutUint32(buf[4:8], uint32(id))
	binary.LittleEndian.PutUint32(buf[8:12], uint32(ptype))
	copy(buf[12:], body)
	_, _ = w.Write(buf)
}

func TestMinecraftRCONAuthAndCommand(t *testing.T) {
	addr, commands := fakeRCONServer(t, "s3cret")

	c, err := NewMinecraftRCONClient(context.Background(), addr, "s3cret")
	if err != nil {
		t.Fatalf("auth with correct password: %v", err)
	}
	defer func() { _ = c.Close() }()

	out, err := c.Command(context.Background(), "list")
	if err != nil || out != "ok" {
		t.Fatalf("command: out=%q err=%v", out, err)
	}
	if got := <-commands; got != "list" {
		t.Fatalf("server received %q", got)
	}
}

func TestMinecraftRCONAuthWrongPassword(t *testing.T) {
	addr, _ := fakeRCONServer(t, "s3cret")

	_, err := NewMinecraftRCONClient(context.Background(), addr, "wrong")
	if err == nil {
		t.Fatal("expected auth failure for wrong password")
	}
}
