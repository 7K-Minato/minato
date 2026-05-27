package rcon

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

type minecraftRCON struct {
	conn net.Conn
}

// NewMinecraftRCONClient creates a new Minecraft RCON client.
func NewMinecraftRCONClient(ctx context.Context, addr, password string) (Client, error) {
	d := &net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial minecraft rcon: %w", err)
	}

	c := &minecraftRCON{conn: conn}

	// Authenticate
	if err := c.auth(password); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("minecraft rcon auth: %w", err)
	}

	return c, nil
}

func (c *minecraftRCON) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *minecraftRCON) Command(ctx context.Context, cmd string) (string, error) {
	if err := c.sendPacket(2, cmd); err != nil {
		return "", fmt.Errorf("send command: %w", err)
	}

	resp, err := c.readPacket()
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	return resp, nil
}

func (c *minecraftRCON) auth(password string) error {
	if err := c.sendPacket(3, password); err != nil {
		return err
	}

	// Read auth response
	id, ptype, body, err := c.readRawPacket()
	if err != nil {
		return err
	}
	if id == -1 || ptype != 2 {
		return fmt.Errorf("auth failed: id=%d type=%d body=%q", id, ptype, body)
	}
	return nil
}

func (c *minecraftRCON) sendPacket(id int32, body string) error {
	bodyBytes := []byte(body)
	length := int32(4 + 4 + len(bodyBytes) + 2) // id + type + body + 2 null bytes

	buf := make([]byte, 4+length)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(length))
	binary.LittleEndian.PutUint32(buf[4:8], uint32(id))
	binary.LittleEndian.PutUint32(buf[8:12], 2) // command type
	copy(buf[12:], bodyBytes)
	buf[12+len(bodyBytes)] = 0
	buf[12+len(bodyBytes)+1] = 0

	_, err := c.conn.Write(buf)
	return err
}

func (c *minecraftRCON) readPacket() (string, error) {
	_, _, body, err := c.readRawPacket()
	return body, err
}

func (c *minecraftRCON) readRawPacket() (id int32, ptype int32, body string, err error) {
	return readRawPacket(c.conn)
}

func readRawPacket(conn net.Conn) (id int32, ptype int32, body string, err error) {
	// Read length
	var length int32
	if err := binary.Read(conn, binary.LittleEndian, &length); err != nil {
		return 0, 0, "", fmt.Errorf("read length: %w", err)
	}
	if length < 10 || length > 4096 {
		return 0, 0, "", fmt.Errorf("invalid packet length: %d", length)
	}

	// Read rest of packet
	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		return 0, 0, "", fmt.Errorf("read packet body: %w", err)
	}

	id = int32(binary.LittleEndian.Uint32(data[0:4]))
	ptype = int32(binary.LittleEndian.Uint32(data[4:8]))

	// Body is null-terminated; find the null byte
	bodyBytes := data[8:]
	for i, b := range bodyBytes {
		if b == 0 {
			body = string(bodyBytes[:i])
			break
		}
	}
	return id, ptype, body, nil
}

// MinecraftClient is a wrapper around a generic Client for Minecraft-specific operations.
type MinecraftClient struct {
	client Client
}

// NewMinecraftClient creates a new MinecraftClient wrapping the given Client.
func NewMinecraftClient(client Client) *MinecraftClient {
	return &MinecraftClient{client: client}
}

// Command delegates to the underlying client.
func (c *MinecraftClient) Command(ctx context.Context, command string) (string, error) {
	return c.client.Command(ctx, command)
}

// Close delegates to the underlying client.
func (c *MinecraftClient) Close() error {
	return c.client.Close()
}

// parseMinecraftPlayerCount parses the output of Minecraft's "list" command.
func parseMinecraftPlayerCount(output string) (online, capacity int) {
	// Expected format: "There are X of a max Y players online: ..."
	var o, c int
	if _, err := fmt.Sscanf(output, "There are %d of a max %d players online", &o, &c); err == nil {
		return o, c
	}
	// Fallback: try to find numbers in the string
	parts := strings.Fields(output)
	for i, p := range parts {
		if p == "are" && i+1 < len(parts) {
			if v, err := strconv.Atoi(parts[i+1]); err == nil {
				o = v
			}
		}
		if p == "max" && i+1 < len(parts) {
			if v, err := strconv.Atoi(parts[i+1]); err == nil {
				c = v
			}
		}
	}
	return o, c
}
