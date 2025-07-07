package agent

import (
	"context"
	"log"

	"github.com/panyam/gocurrent"
	protos "github.com/panyam/devloop/gen/go/devloop/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AgentService struct {
	protos.UnimplementedAgentServiceServer
	orchestrator *Orchestrator
}

func NewAgentService(orch *Orchestrator) *AgentService {
	out := &AgentService{
		orchestrator: orch,
	}
	return out
}

// Get the configs of this agent
func (s *AgentService) GetConfig(ctx context.Context, req *protos.GetConfigRequest) (resp *protos.GetConfigResponse, err error) {
	o := s.orchestrator
	resp = &protos.GetConfigResponse{Config: o.Config}
	return
}

func (s *AgentService) GetRule(ctx context.Context, req *protos.GetRuleRequest) (resp *protos.GetRuleResponse, err error) {
	o := s.orchestrator
	log.Printf("[devloop] Received GetRuleStatus request for rule %q", req.GetRuleName())

	rule, _, ok := o.GetRuleStatus(req.GetRuleName())
	if !ok {
		err = status.Errorf(codes.NotFound, "Rule not found")
	} else {
		resp = &protos.GetRuleResponse{
			Rule: rule,
		}
	}
	return
}

func (s *AgentService) ListWatchedPaths(ctx context.Context, req *protos.ListWatchedPathsRequest) (resp *protos.ListWatchedPathsResponse, err error) {
	o := s.orchestrator
	log.Printf("[devloop] Received ListWatchedPaths request")

	paths := o.GetWatchedPaths()
	resp = &protos.ListWatchedPathsResponse{
		Paths: paths,
	}
	return
}

func (s *AgentService) StreamLogs(req *protos.StreamLogsRequest, stream grpc.ServerStreamingServer[protos.StreamLogsResponse]) error {
	log.Printf("[devloop] Received StreamLogs request for rule %q with filter %q", req.GetRuleName(), req.GetFilter())

	// Create a gocurrent Writer that forwards messages to the gRPC stream
	writer := gocurrent.NewWriter(func(response *protos.StreamLogsResponse) error {
		return stream.Send(response)
	})
	defer writer.Stop()

	// Call the orchestrator's StreamLogs method with our writer
	err := s.orchestrator.StreamLogs(req.GetRuleName(), req.GetFilter(), writer)
	if err != nil {
		log.Printf("[devloop] Error streaming logs: %v", err)
		return status.Errorf(codes.Internal, "failed to stream logs: %v", err)
	}

	// Wait for the writer to complete (or for context cancellation)
	select {
	case <-stream.Context().Done():
		log.Printf("[devloop] StreamLogs cancelled by client")
		return stream.Context().Err()
	case err := <-writer.ClosedChan():
		log.Printf("[devloop] StreamLogs writer closed")
		if err != nil {
			return status.Errorf(codes.Internal, "writer error: %v", err)
		}
		return nil
	}
}

func (s *AgentService) TriggerRule(ctx context.Context, req *protos.TriggerRuleRequest) (resp *protos.TriggerRuleResponse, err error) {
	o := s.orchestrator
	log.Printf("[devloop] Received TriggerRule request for rule %q", req.GetRuleName())

	err = o.TriggerRule(req.GetRuleName())
	resp = &protos.TriggerRuleResponse{
		Success: err == nil,
	}
	return
}
