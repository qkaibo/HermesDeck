package bridge

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/hermesdeck/internal/tools"
	pb "github.com/hermesdeck/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	addr      string
	conn      *grpc.ClientConn
	client    pb.HermesDeckBridgeClient
	toolReg   *tools.Registry
	mu        sync.RWMutex
	connected bool
}

func NewClient(addr string, toolReg *tools.Registry) *Client {
	return &Client{addr: addr, toolReg: toolReg}
}

func (c *Client) Connect(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, c.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.conn = conn
	c.client = pb.NewHermesDeckBridgeClient(conn)
	c.connected = true
	c.mu.Unlock()
	return nil
}

func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *Client) RegisterTools(ctx context.Context, defs []tools.ToolDefinition) error {
	if !c.IsConnected() {
		return nil
	}
	var toolsMsg []*pb.ToolDefinition
	for _, d := range defs {
		schemaBytes, _ := json.Marshal(d.Parameters)
		toolsMsg = append(toolsMsg, &pb.ToolDefinition{
			Name:               d.Name,
			Description:        d.Description,
			ParametersJsonSchema: string(schemaBytes),
		})
	}
	_, err := c.client.RegisterTools(ctx, &pb.ToolRegistry{Tools: toolsMsg})
	return err
}

func (c *Client) ProcessMessage(ctx context.Context, sessionID, msg string) (<-chan *pb.MessageResponse, error) {
	if !c.IsConnected() {
		return nil, nil
	}
	stream, err := c.client.ProcessMessage(ctx, &pb.MessageRequest{
		SessionId: sessionID, UserMessage: msg, Stream: true,
	})
	if err != nil {
		return nil, err
	}
	respCh := make(chan *pb.MessageResponse, 64)
	go func() {
		defer close(respCh)
		for {
			resp, err := stream.Recv()
			if err != nil {
				return
			}
			respCh <- resp
		}
	}()
	return respCh, nil
}

func (c *Client) HealthCheck(ctx context.Context) error {
	if !c.IsConnected() {
		return nil
	}
	_, err := c.client.HealthCheck(ctx, &pb.HealthRequest{Service: "go"})
	return err
}

// GetClient returns the underlying gRPC bridge client.
// Returns nil if not connected.
func (c *Client) GetClient() pb.HermesDeckBridgeClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		c.conn.Close()
	}
	c.connected = false
	log.Println("Python sidecar connection closed.")
}
