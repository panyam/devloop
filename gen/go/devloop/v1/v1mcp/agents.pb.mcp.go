// Code generated by protoc-gen-mcp-go. DO NOT EDIT.
// source: devloop/v1/agents.proto

package v1mcp

import (
	v1 "github.com/panyam/devloop/gen/go/devloop/v1"
)

import (
	"connectrpc.com/connect"
	"context"
	"encoding/json"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	grpc "google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
)

var (
	AgentService_GetConfigTool        = mcp.Tool{Name: "get_config", Description: "Retrieve the complete devloop configuration for a project to understand\navailable rules, commands, file watch patterns, and project settings.\n\nEssential information provided:\n- Available build/test rules (rules[].name)\n- Commands executed by each rule (rules[].commands)\n- File patterns that trigger each rule (rules[].watch patterns)\n- Project settings like colors, logging, debouncing\n\nUsage Examples:\n- Discover available rules: Parse rules[].name from response\n- Find test commands: Look for rules with \"test\" in name or commands\n- Understand file triggers: Examine rules[].watch.patterns\n\nResponse Format: JSON string containing the complete .devloop.yaml content\nwith resolved settings and rule definitions.\nmcp_tool_name:get_config\n", InputSchema: mcp.ToolInputSchema{Type: "", Properties: map[string]interface{}(nil), Required: []string(nil)}, RawInputSchema: json.RawMessage{0x7b, 0x22, 0x70, 0x72, 0x6f, 0x70, 0x65, 0x72, 0x74, 0x69, 0x65, 0x73, 0x22, 0x3a, 0x7b, 0x7d, 0x2c, 0x22, 0x72, 0x65, 0x71, 0x75, 0x69, 0x72, 0x65, 0x64, 0x22, 0x3a, 0x5b, 0x5d, 0x2c, 0x22, 0x74, 0x79, 0x70, 0x65, 0x22, 0x3a, 0x22, 0x6f, 0x62, 0x6a, 0x65, 0x63, 0x74, 0x22, 0x7d}}
	AgentService_GetRuleTool          = mcp.Tool{Name: "get_rule_status", Description: "Get the current execution status and history of a specific rule.\nUse this to monitor build/test progress and check for failures.\n\nStatus Information Provided:\n- Whether the rule is currently running\n- When the current/last execution started\n- Result of the last execution (SUCCESS, FAILED, RUNNING, IDLE)\n- Execution history timestamps\n\nCommon Use Cases:\n- Check if a build is still running after triggering\n- Verify if tests passed or failed\n- Monitor long-running development servers\n- Debug why a rule isn't executing\n\nReturns: Detailed status including timing and execution results\nmcp_tool_name:get_rule_status\n", InputSchema: mcp.ToolInputSchema{Type: "", Properties: map[string]interface{}(nil), Required: []string(nil)}, RawInputSchema: json.RawMessage{0x7b, 0x22, 0x70, 0x72, 0x6f, 0x70, 0x65, 0x72, 0x74, 0x69, 0x65, 0x73, 0x22, 0x3a, 0x7b, 0x22, 0x72, 0x75, 0x6c, 0x65, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x22, 0x3a, 0x7b, 0x22, 0x74, 0x79, 0x70, 0x65, 0x22, 0x3a, 0x22, 0x73, 0x74, 0x72, 0x69, 0x6e, 0x67, 0x22, 0x7d, 0x7d, 0x2c, 0x22, 0x72, 0x65, 0x71, 0x75, 0x69, 0x72, 0x65, 0x64, 0x22, 0x3a, 0x5b, 0x5d, 0x2c, 0x22, 0x74, 0x79, 0x70, 0x65, 0x22, 0x3a, 0x22, 0x6f, 0x62, 0x6a, 0x65, 0x63, 0x74, 0x22, 0x7d}}
	AgentService_ListWatchedPathsTool = mcp.Tool{Name: "list_watched_paths", Description: "List all file glob patterns being monitored by a project for automatic rule triggering.\nUse this to understand what files cause rebuilds and which rules will execute.\n\nPattern Information:\n- All include/exclude patterns from all rules combined\n- Glob syntax: **/*.go, src/**/*.js, **/test_*.py, etc.\n- Patterns are resolved relative to the project root\n\nCommon Use Cases:\n- Understand what file changes trigger builds\n- Debug why edits aren't triggering rules\n- Plan file organization to optimize build triggers\n- Analyze project structure and dependencies\n\nReturns: Array of glob patterns currently being watched\nmcp_tool_name:list_watched_paths\n", InputSchema: mcp.ToolInputSchema{Type: "", Properties: map[string]interface{}(nil), Required: []string(nil)}, RawInputSchema: json.RawMessage{0x7b, 0x22, 0x70, 0x72, 0x6f, 0x70, 0x65, 0x72, 0x74, 0x69, 0x65, 0x73, 0x22, 0x3a, 0x7b, 0x7d, 0x2c, 0x22, 0x72, 0x65, 0x71, 0x75, 0x69, 0x72, 0x65, 0x64, 0x22, 0x3a, 0x5b, 0x5d, 0x2c, 0x22, 0x74, 0x79, 0x70, 0x65, 0x22, 0x3a, 0x22, 0x6f, 0x62, 0x6a, 0x65, 0x63, 0x74, 0x22, 0x7d}}
	AgentService_TriggerRuleTool      = mcp.Tool{Name: "trigger_rule", Description: "Manually execute a specific rule to run builds, tests, or other commands.\nThis bypasses file watching and immediately starts the rule's command sequence.\n\nTrigger Behavior:\n- Terminates any currently running instance of the rule\n- Executes all commands in the rule definition sequentially\n- Updates rule status to RUNNING, then SUCCESS/FAILED based on results\n- Generates log output that can be retrieved via streaming endpoints\n\nCommon Use Cases:\n- Run builds on demand (\"trigger the backend build\")\n- Execute test suites (\"run the test rule\")\n- Restart development servers (\"trigger the dev-server rule\")\n- Force regeneration (\"trigger the protobuf rule\")\n\nReturns: Immediate response indicating if trigger was accepted\nUse GetRule() to monitor actual execution progress\nmcp_tool_name:trigger_rule\n", InputSchema: mcp.ToolInputSchema{Type: "", Properties: map[string]interface{}(nil), Required: []string(nil)}, RawInputSchema: json.RawMessage{0x7b, 0x22, 0x70, 0x72, 0x6f, 0x70, 0x65, 0x72, 0x74, 0x69, 0x65, 0x73, 0x22, 0x3a, 0x7b, 0x22, 0x72, 0x75, 0x6c, 0x65, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x22, 0x3a, 0x7b, 0x22, 0x74, 0x79, 0x70, 0x65, 0x22, 0x3a, 0x22, 0x73, 0x74, 0x72, 0x69, 0x6e, 0x67, 0x22, 0x7d, 0x7d, 0x2c, 0x22, 0x72, 0x65, 0x71, 0x75, 0x69, 0x72, 0x65, 0x64, 0x22, 0x3a, 0x5b, 0x5d, 0x2c, 0x22, 0x74, 0x79, 0x70, 0x65, 0x22, 0x3a, 0x22, 0x6f, 0x62, 0x6a, 0x65, 0x63, 0x74, 0x22, 0x7d}}
)

// AgentServiceServer is compatible with the grpc-go server interface.
type AgentServiceServer interface {
	GetConfig(ctx context.Context, req *v1.GetConfigRequest) (*v1.GetConfigResponse, error)
	GetRule(ctx context.Context, req *v1.GetRuleRequest) (*v1.GetRuleResponse, error)
	ListWatchedPaths(ctx context.Context, req *v1.ListWatchedPathsRequest) (*v1.ListWatchedPathsResponse, error)
	TriggerRule(ctx context.Context, req *v1.TriggerRuleRequest) (*v1.TriggerRuleResponse, error)
}

func RegisterAgentServiceHandler(s *mcpserver.MCPServer, srv AgentServiceServer) {
	s.AddTool(AgentService_GetConfigTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req v1.GetConfigRequest

		message := request.Params.Arguments

		marshaled, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}

		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(marshaled, &req); err != nil {
			return nil, err
		}

		resp, err := srv.GetConfig(ctx, &req)
		if err != nil {
			return nil, err
		}

		marshaled, err = (protojson.MarshalOptions{UseProtoNames: true, EmitDefaultValues: true}).Marshal(resp)
		if err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(string(marshaled)), nil
	})
	s.AddTool(AgentService_GetRuleTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req v1.GetRuleRequest

		message := request.Params.Arguments

		marshaled, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}

		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(marshaled, &req); err != nil {
			return nil, err
		}

		resp, err := srv.GetRule(ctx, &req)
		if err != nil {
			return nil, err
		}

		marshaled, err = (protojson.MarshalOptions{UseProtoNames: true, EmitDefaultValues: true}).Marshal(resp)
		if err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(string(marshaled)), nil
	})
	s.AddTool(AgentService_ListWatchedPathsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req v1.ListWatchedPathsRequest

		message := request.Params.Arguments

		marshaled, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}

		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(marshaled, &req); err != nil {
			return nil, err
		}

		resp, err := srv.ListWatchedPaths(ctx, &req)
		if err != nil {
			return nil, err
		}

		marshaled, err = (protojson.MarshalOptions{UseProtoNames: true, EmitDefaultValues: true}).Marshal(resp)
		if err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(string(marshaled)), nil
	})
	s.AddTool(AgentService_TriggerRuleTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req v1.TriggerRuleRequest

		message := request.Params.Arguments

		marshaled, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}

		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(marshaled, &req); err != nil {
			return nil, err
		}

		resp, err := srv.TriggerRule(ctx, &req)
		if err != nil {
			return nil, err
		}

		marshaled, err = (protojson.MarshalOptions{UseProtoNames: true, EmitDefaultValues: true}).Marshal(resp)
		if err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(string(marshaled)), nil
	})
}

// AgentServiceClient is compatible with the grpc-go client interface.
type AgentServiceClient interface {
	GetConfig(ctx context.Context, req *v1.GetConfigRequest, opts ...grpc.CallOption) (*v1.GetConfigResponse, error)
	GetRule(ctx context.Context, req *v1.GetRuleRequest, opts ...grpc.CallOption) (*v1.GetRuleResponse, error)
	ListWatchedPaths(ctx context.Context, req *v1.ListWatchedPathsRequest, opts ...grpc.CallOption) (*v1.ListWatchedPathsResponse, error)
	TriggerRule(ctx context.Context, req *v1.TriggerRuleRequest, opts ...grpc.CallOption) (*v1.TriggerRuleResponse, error)
}

// ConnectAgentServiceClient is compatible with the connectrpc-go client interface.
type ConnectAgentServiceClient interface {
	GetConfig(ctx context.Context, req *connect.Request[v1.GetConfigRequest]) (*connect.Response[v1.GetConfigResponse], error)
	GetRule(ctx context.Context, req *connect.Request[v1.GetRuleRequest]) (*connect.Response[v1.GetRuleResponse], error)
	ListWatchedPaths(ctx context.Context, req *connect.Request[v1.ListWatchedPathsRequest]) (*connect.Response[v1.ListWatchedPathsResponse], error)
	TriggerRule(ctx context.Context, req *connect.Request[v1.TriggerRuleRequest]) (*connect.Response[v1.TriggerRuleResponse], error)
}

// ForwardToConnectAgentServiceClient registers a connectrpc client, to forward MCP calls to it.
func ForwardToConnectAgentServiceClient(s *mcpserver.MCPServer, client ConnectAgentServiceClient) {
	s.AddTool(AgentService_GetConfigTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req v1.GetConfigRequest

		message := request.Params.Arguments

		marshaled, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}

		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(marshaled, &req); err != nil {
			return nil, err
		}

		resp, err := client.GetConfig(ctx, connect.NewRequest(&req))
		if err != nil {
			return nil, err
		}

		marshaled, err = (protojson.MarshalOptions{UseProtoNames: true, EmitDefaultValues: true}).Marshal(resp.Msg)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(string(marshaled)), nil
	})
	s.AddTool(AgentService_GetRuleTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req v1.GetRuleRequest

		message := request.Params.Arguments

		marshaled, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}

		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(marshaled, &req); err != nil {
			return nil, err
		}

		resp, err := client.GetRule(ctx, connect.NewRequest(&req))
		if err != nil {
			return nil, err
		}

		marshaled, err = (protojson.MarshalOptions{UseProtoNames: true, EmitDefaultValues: true}).Marshal(resp.Msg)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(string(marshaled)), nil
	})
	s.AddTool(AgentService_ListWatchedPathsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req v1.ListWatchedPathsRequest

		message := request.Params.Arguments

		marshaled, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}

		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(marshaled, &req); err != nil {
			return nil, err
		}

		resp, err := client.ListWatchedPaths(ctx, connect.NewRequest(&req))
		if err != nil {
			return nil, err
		}

		marshaled, err = (protojson.MarshalOptions{UseProtoNames: true, EmitDefaultValues: true}).Marshal(resp.Msg)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(string(marshaled)), nil
	})
	s.AddTool(AgentService_TriggerRuleTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req v1.TriggerRuleRequest

		message := request.Params.Arguments

		marshaled, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}

		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(marshaled, &req); err != nil {
			return nil, err
		}

		resp, err := client.TriggerRule(ctx, connect.NewRequest(&req))
		if err != nil {
			return nil, err
		}

		marshaled, err = (protojson.MarshalOptions{UseProtoNames: true, EmitDefaultValues: true}).Marshal(resp.Msg)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(string(marshaled)), nil
	})
}

// ForwardToAgentServiceClient registers a gRPC client, to forward MCP calls to it.
func ForwardToAgentServiceClient(s *mcpserver.MCPServer, client AgentServiceClient) {
	s.AddTool(AgentService_GetConfigTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req v1.GetConfigRequest

		message := request.Params.Arguments

		marshaled, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}

		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(marshaled, &req); err != nil {
			return nil, err
		}

		resp, err := client.GetConfig(ctx, &req)
		if err != nil {
			return nil, err
		}

		marshaled, err = (protojson.MarshalOptions{UseProtoNames: true, EmitDefaultValues: true}).Marshal(resp)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(string(marshaled)), nil
	})
	s.AddTool(AgentService_GetRuleTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req v1.GetRuleRequest

		message := request.Params.Arguments

		marshaled, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}

		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(marshaled, &req); err != nil {
			return nil, err
		}

		resp, err := client.GetRule(ctx, &req)
		if err != nil {
			return nil, err
		}

		marshaled, err = (protojson.MarshalOptions{UseProtoNames: true, EmitDefaultValues: true}).Marshal(resp)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(string(marshaled)), nil
	})
	s.AddTool(AgentService_ListWatchedPathsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req v1.ListWatchedPathsRequest

		message := request.Params.Arguments

		marshaled, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}

		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(marshaled, &req); err != nil {
			return nil, err
		}

		resp, err := client.ListWatchedPaths(ctx, &req)
		if err != nil {
			return nil, err
		}

		marshaled, err = (protojson.MarshalOptions{UseProtoNames: true, EmitDefaultValues: true}).Marshal(resp)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(string(marshaled)), nil
	})
	s.AddTool(AgentService_TriggerRuleTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req v1.TriggerRuleRequest

		message := request.Params.Arguments

		marshaled, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}

		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(marshaled, &req); err != nil {
			return nil, err
		}

		resp, err := client.TriggerRule(ctx, &req)
		if err != nil {
			return nil, err
		}

		marshaled, err = (protojson.MarshalOptions{UseProtoNames: true, EmitDefaultValues: true}).Marshal(resp)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(string(marshaled)), nil
	})
}
