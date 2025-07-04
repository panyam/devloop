// Package mcp provides Model Context Protocol (MCP) integration for devloop.
//
// This package implements MCP server functionality that allows devloop to be
// used as an MCP tool by AI assistants and other external clients. The MCP
// integration provides structured access to devloop's capabilities through
// the standardized MCP protocol.
//
// # MCP Server
//
// The MCP server exposes devloop functionality as MCP tools:
// - Project configuration management
// - Rule triggering and status monitoring
// - File content reading
// - Real-time log streaming
//
// # Usage
//
// The MCP server runs over HTTP/SSE transport when enabled:
//
//	devloop --enable-mcp --mcp-port 3000  # MCP server on HTTP port 3000
//
// # Integration
//
// AI assistants can use devloop through MCP by:
// - Connecting to the HTTP-based MCP server via SSE transport
// - Calling available MCP tools for project management
// - Receiving real-time updates through the MCP protocol
//
// # Tools Available
//
// - get_config: Retrieve project configuration
// - trigger_rule: Execute specific automation rules
// - get_rule_status: Check rule execution status
// - list_watched_paths: Get monitored file patterns
// - read_file_content: Access file contents
// - stream_logs: Receive real-time log updates
package mcp

import (
	"context"
	"fmt"
	"log"

	"github.com/panyam/devloop/gateway"
	pb "github.com/panyam/devloop/gen/go/protos/devloop/v1"
)

// SelectiveGatewayAdapter exposes only essential devloop operations as MCP tools
type SelectiveGatewayAdapter struct {
	orchestrator gateway.Orchestrator
}

// ListProjects implements the MCP ListProjects tool
func (s *SelectiveGatewayAdapter) ListProjects(ctx context.Context, req *pb.ListProjectsRequest) (*pb.ListProjectsResponse, error) {
	// For now, return the current project information
	// In the future, this could connect to a gateway to list multiple projects
	config := s.orchestrator.GetConfig()

	// Get project ID from config settings or generate one
	projectID := config.Settings.ProjectID
	if projectID == "" {
		projectID = "devloop-project" // fallback
	}

	return &pb.ListProjectsResponse{
		Projects: []*pb.ListProjectsResponse_Project{
			{
				ProjectId:   projectID,
				ProjectRoot: ".", // Current directory
				Status:      "CONNECTED",
			},
		},
	}, nil
}

// GetConfig implements the MCP GetConfig tool
func (s *SelectiveGatewayAdapter) GetConfig(ctx context.Context, req *pb.GetConfigRequest) (*pb.GetConfigResponse, error) {
	log.Printf("[mcp] GetConfig called for project: %s", req.GetProjectId())

	config := s.orchestrator.GetConfig()

	// Marshal config to JSON
	configJSON, err := marshalConfig(config)
	if err != nil {
		return nil, err
	}

	return &pb.GetConfigResponse{
		ConfigJson: configJSON,
	}, nil
}

// GetRuleStatus implements the MCP GetRuleStatus tool
func (s *SelectiveGatewayAdapter) GetRuleStatus(ctx context.Context, req *pb.GetRuleStatusRequest) (*pb.GetRuleStatusResponse, error) {
	log.Printf("[mcp] GetRuleStatus called for project: %s, rule: %s", req.GetProjectId(), req.GetRuleName())

	status, ok := s.orchestrator.GetRuleStatus(req.GetRuleName())
	if !ok {
		return &pb.GetRuleStatusResponse{}, nil
	}

	return &pb.GetRuleStatusResponse{
		RuleStatus: &pb.RuleStatus{
			ProjectId:       req.GetProjectId(),
			RuleName:        status.RuleName,
			IsRunning:       status.IsRunning,
			StartTime:       status.StartTime.UnixMilli(),
			LastBuildTime:   status.LastBuildTime.UnixMilli(),
			LastBuildStatus: status.LastBuildStatus,
		},
	}, nil
}

// TriggerRuleClient implements the MCP TriggerRule tool
func (s *SelectiveGatewayAdapter) TriggerRuleClient(ctx context.Context, req *pb.TriggerRuleClientRequest) (*pb.TriggerRuleClientResponse, error) {
	log.Printf("[mcp] TriggerRule called for project: %s, rule: %s", req.GetProjectId(), req.GetRuleName())

	err := s.orchestrator.TriggerRule(req.GetRuleName())
	if err != nil {
		return &pb.TriggerRuleClientResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	return &pb.TriggerRuleClientResponse{
		Success: true,
		Message: "Rule triggered successfully",
	}, nil
}

// ReadFileContent implements the MCP ReadFileContent tool
func (s *SelectiveGatewayAdapter) ReadFileContent(ctx context.Context, req *pb.ReadFileContentRequest) (*pb.ReadFileContentResponse, error) {
	log.Printf("[mcp] ReadFileContent called for project: %s, path: %s", req.GetProjectId(), req.GetPath())

	content, err := s.orchestrator.ReadFileContent(req.GetPath())
	if err != nil {
		return nil, err
	}

	return &pb.ReadFileContentResponse{
		Content: content,
	}, nil
}

// ListWatchedPaths implements the MCP ListWatchedPaths tool
func (s *SelectiveGatewayAdapter) ListWatchedPaths(ctx context.Context, req *pb.ListWatchedPathsRequest) (*pb.ListWatchedPathsResponse, error) {
	log.Printf("[mcp] ListWatchedPaths called for project: %s", req.GetProjectId())

	paths := s.orchestrator.GetWatchedPaths()
	return &pb.ListWatchedPathsResponse{
		Paths: paths,
	}, nil
}

// Helper function to marshal config to JSON
func marshalConfig(config *gateway.Config) ([]byte, error) {
	// You might want to use a specific JSON marshaling approach here
	// For now, we'll use a simple approach
	configStr := "{\n"
	configStr += "  \"settings\": {\n"
	if config.Settings.ProjectID != "" {
		configStr += fmt.Sprintf("    \"project_id\": \"%s\",\n", config.Settings.ProjectID)
	}
	configStr += fmt.Sprintf("    \"prefix_logs\": %t,\n", config.Settings.PrefixLogs)
	configStr += fmt.Sprintf("    \"prefix_max_length\": %d,\n", config.Settings.PrefixMaxLength)
	configStr += fmt.Sprintf("    \"color_logs\": %t\n", config.Settings.ColorLogs)
	configStr += "  },\n"
	configStr += "  \"rules\": [\n"
	for i, rule := range config.Rules {
		configStr += "    {\n"
		configStr += fmt.Sprintf("      \"name\": \"%s\",\n", rule.Name)
		configStr += fmt.Sprintf("      \"commands\": %q\n", rule.Commands)
		configStr += "    }"
		if i < len(config.Rules)-1 {
			configStr += ","
		}
		configStr += "\n"
	}
	configStr += "  ]\n"
	configStr += "}"

	return []byte(configStr), nil
}
