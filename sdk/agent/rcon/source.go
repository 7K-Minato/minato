package rcon

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
)

type sourceRCON struct {
	conn net.Conn
}

// NewSourceRCONClient creates a new Source engine RCON client (for CS2, etc.).
func NewSourceRCONClient(ctx context.Context, addr, password string) (Client, error) {
	d := &net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial source rcon: %w", err)
	}

	c := &sourceRCON{conn: conn}

	// Authenticate
	if err := c.auth(password); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("source rcon auth: %w", err)
	}

	return c, nil
}

func (c *sourceRCON) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *sourceRCON) Command(ctx context.Context, cmd string) (string, error) {
	if err := c.sendPacket(2, cmd); err != nil {
		return "", fmt.Errorf("send command: %w", err)
	}

	resp, err := c.readPacket()
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	return resp, nil
}

func (c *sourceRCON) auth(password string) error {
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

func (c *sourceRCON) sendPacket(id int32, body string) error {
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

func (c *sourceRCON) readPacket() (string, error) {
	_, _, body, err := c.readRawPacket()
	return body, err
}

func (c *sourceRCON) readRawPacket() (id int32, ptype int32, body string, err error) {
	return readRawPacket(c.conn)
}

// SourceClient is a wrapper around a generic Client for Source engine-specific operations.
type SourceClient struct {
	client Client
}

// NewSourceClient creates a new SourceClient wrapping the given Client.
func NewSourceClient(client Client) *SourceClient {
	return &SourceClient{client: client}
}

// Command delegates to the underlying client.
func (c *SourceClient) Command(ctx context.Context, command string) (string, error) {
	return c.client.Command(ctx, command)
}

// Close delegates to the underlying client.
func (c *SourceClient) Close() error {
	return c.client.Close()
}

// parseSourcePlayerCount parses the output of Source engine's "status" or "listplayers" command.
func parseSourcePlayerCount(output string) (online, capacity int) {
	// Try to parse "players : X (Y max)" from status output
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "players") {
			var o, c int
			if _, err := fmt.Sscanf(line, "players : %d (%d max)", &o, &c); err == nil {
				return o, c
			}
		}
		// Alternative format: "# userid name ..."
		if strings.HasPrefix(line, "#") && !strings.Contains(line, "userid") {
			online++
		}
	}
	return online, capacity
}

// parseSourcePlayerList parses player names from Source engine status output.
func parseSourcePlayerList(output string) []string {
	var players []string
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") && !strings.Contains(line, "userid") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				players = append(players, fields[2])
			}
		}
	}
	return players
}
