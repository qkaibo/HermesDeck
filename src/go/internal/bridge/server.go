package bridge

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"time"

	"github.com/hermesdeck/internal/session"
	"github.com/hermesdeck/internal/tools"
	pb "github.com/hermesdeck/pkg/proto"
	"google.golang.org/grpc"
)

type Server struct {
	pb.UnimplementedHermesDeckBridgeServer
	addr       string
	toolReg    *tools.Registry
	sessionMgr *session.Manager
	server     *grpc.Server
}

func NewServer(addr string, toolReg *tools.Registry, sessionMgr *session.Manager) *Server {
	return &Server{addr: addr, toolReg: toolReg, sessionMgr: sessionMgr}
}

func (s *Server) Start(ctx context.Context) error {
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.server = grpc.NewServer()
	pb.RegisterHermesDeckBridgeServer(s.server, s)
	log.Printf("gRPC server listening on %s", s.addr)
	go func() {
		<-ctx.Done()
		s.server.GracefulStop()
	}()
	return s.server.Serve(lis)
}

func (s *Server) ExecuteTool(ctx context.Context, req *pb.ToolRequest) (*pb.ToolResponse, error) {
	tool, ok := s.toolReg.Get(req.ToolName)
	if !ok {
		return &pb.ToolResponse{
			ToolCallId: req.ToolCallId, IsError: true,
			ErrorMessage: "unknown tool: " + req.ToolName,
		}, nil
	}
	var args map[string]interface{}
	if req.ArgumentsJson != "" {
		if err := json.Unmarshal([]byte(req.ArgumentsJson), &args); err != nil {
			return &pb.ToolResponse{
				ToolCallId: req.ToolCallId, IsError: true,
				ErrorMessage: err.Error(),
			}, nil
		}
	}
	result, err := tool.Handler(ctx, args)
	if err != nil {
		return &pb.ToolResponse{
			ToolCallId: req.ToolCallId, IsError: true,
			ErrorMessage: err.Error(),
		}, nil
	}
	resultJSON, _ := json.Marshal(result)
	return &pb.ToolResponse{
		ToolCallId: req.ToolCallId, ResultJson: string(resultJSON),
	}, nil
}

func (s *Server) HealthCheck(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{
		Service: "go", Status: "alive",
		Timestamp: time.Now().Unix(), Version: "0.1.0",
	}, nil
}
